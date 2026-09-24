package workspace

import (
	"errors"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	// As operações estruturais são deliberadamente limitadas e não aceitam
	// limites fornecidos remotamente nesta fatia.
	MaxStructuralDepth   = 32
	MaxStructuralEntries = 4096
	MaxStructuralBytes   = 64 << 20
)

var (
	ErrCrossDevice       = errors.New("workspace move crosses filesystems")
	ErrDirectoryNotEmpty = errors.New("workspace directory is not empty")
	ErrStructuralLimit   = errors.New("workspace structural operation limit exceeded")
	ErrOperationUnknown  = errors.New("workspace operation result is unknown")
	ErrUnsupportedType   = errors.New("workspace path type is not supported")
)

type CopyResult struct {
	Source        string   `json:"source"`
	Destination   string   `json:"destination"`
	Kind          FileKind `json:"kind"`
	FilesCopied   int      `json:"files_copied"`
	EntriesCopied int      `json:"entries_copied"`
	BytesCopied   int64    `json:"bytes_copied"`
	Status        string   `json:"status"`
	Partial       bool     `json:"partial,omitempty"`
	Cleanup       string   `json:"cleanup,omitempty"`
}

type MoveResult struct {
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Kind        FileKind `json:"kind"`
	Status      string   `json:"status"`
}

type DeleteResult struct {
	Path   string   `json:"path"`
	Kind   FileKind `json:"kind"`
	Status string   `json:"status"`
}

func (s *Session) Copy(source, destination string) (result CopyResult, err error) {
	result = CopyResult{Source: source, Destination: destination, Cleanup: "not_required"}
	if err := validateStructuralPair(source, destination); err != nil {
		return result, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return result, ErrClosed
	}

	sourceParent, sourceName, err := s.openParentLocked(source)
	if err != nil {
		return result, err
	}
	if sourceParent != s.rootFD {
		defer unix.Close(sourceParent)
	}
	sourceStat, err := statAtNoFollow(sourceParent, sourceName)
	if err != nil {
		return result, err
	}
	result.Kind = fileKind(sourceStat.Mode)
	if result.Kind == FileKindSymlink {
		return result, ErrUnsafePath
	}
	if result.Kind != FileKindRegular && result.Kind != FileKindDirectory {
		return result, ErrUnsupportedType
	}

	destinationParent, destinationName, err := s.openParentLocked(destination)
	if err != nil {
		return result, err
	}
	if destinationParent != s.rootFD {
		defer unix.Close(destinationParent)
	}
	if existing, statErr := statAtNoFollow(destinationParent, destinationName); statErr == nil {
		if fileKind(existing.Mode) == FileKindSymlink {
			return result, ErrUnsafePath
		}
		return result, ErrPathExists
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return result, statErr
	}

	var destinationStat unix.Stat_t
	if result.Kind == FileKindRegular {
		if sourceStat.Size > MaxStructuralBytes {
			return result, ErrStructuralLimit
		}
		if err := s.copyRegularEntryLocked(sourceParent, sourceName, destinationParent, destinationName, &result); err != nil {
			if destinationStat, statErr := statAtNoFollow(destinationParent, destinationName); statErr == nil {
				return s.copyFailureLocked(result, destinationParent, destinationName, destinationStat, true, err)
			} else if !errors.Is(statErr, os.ErrNotExist) {
				result.Status = "partial"
				result.Partial = true
				result.Cleanup = "unknown"
				return result, errors.Join(err, statErr, ErrOperationUnknown)
			}
			return result, err
		}
		result.Status = "copied"
		return result, nil
	}

	if err := unix.Mkdirat(destinationParent, destinationName, 0700); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return result, ErrPathExists
		}
		return result, normalizePathError(err)
	}
	if destinationStat, err = statAtNoFollow(destinationParent, destinationName); err != nil {
		result.Status = "partial"
		result.Partial = true
		result.Cleanup = "unknown"
		return result, errors.Join(err, ErrOperationUnknown)
	}
	result.EntriesCopied = 1
	destinationCreated := true
	sourceFD, err := unix.Openat(sourceParent, sourceName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == nil {
		destinationFD, openDestErr := unix.Openat(destinationParent, destinationName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if openDestErr != nil {
			err = normalizePathError(openDestErr)
			_ = unix.Close(sourceFD)
		} else {
			err = s.copyDirectoryContentsLocked(sourceFD, destinationFD, 0, &result)
			_ = unix.Close(destinationFD)
		}
	} else {
		err = normalizePathError(err)
	}
	if err != nil {
		return s.copyFailureLocked(result, destinationParent, destinationName, destinationStat, destinationCreated, err)
	}
	result.Status = "copied"
	return result, nil
}

func (s *Session) copyFailureLocked(result CopyResult, parentFD int, name string, expected unix.Stat_t, created bool, operationErr error) (CopyResult, error) {
	if !created {
		result.Status = "failed"
		result.Cleanup = "not_required"
		return result, operationErr
	}
	if cleanupErr := removeCreatedEntryLocked(parentFD, name, expected); cleanupErr != nil {
		result.Status = "partial"
		result.Partial = true
		result.Cleanup = "unknown"
		return result, errors.Join(operationErr, cleanupErr, ErrOperationUnknown)
	}
	result.Status = "failed"
	result.Cleanup = "confirmed"
	return result, operationErr
}

func (s *Session) copyDirectoryContentsLocked(sourceFD, destinationFD, depth int, result *CopyResult) error {
	directory := os.NewFile(uintptr(sourceFD), "workspace-copy-source")
	defer directory.Close()
	names, err := directory.Readdirnames(MaxStructuralEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(names) > MaxStructuralEntries {
		return ErrStructuralLimit
	}
	sort.Strings(names)
	for _, name := range names {
		if name == ".git" {
			continue
		}
		if result.EntriesCopied >= MaxStructuralEntries {
			return ErrStructuralLimit
		}
		if !validDirectoryEntryName(name) {
			return ErrInvalidPath
		}
		stat, err := statAtNoFollow(sourceFD, name)
		if err != nil {
			return err
		}
		kind := fileKind(stat.Mode)
		switch kind {
		case FileKindRegular:
			if err := s.copyRegularEntryLocked(sourceFD, name, destinationFD, name, result); err != nil {
				return err
			}
		case FileKindDirectory:
			if depth >= MaxStructuralDepth {
				return ErrStructuralLimit
			}
			if err := unix.Mkdirat(destinationFD, name, 0700); err != nil {
				if errors.Is(err, unix.EEXIST) {
					return ErrPathExists
				}
				return normalizePathError(err)
			}
			result.EntriesCopied++
			sourceChild, err := unix.Openat(sourceFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return normalizePathError(err)
			}
			destinationChild, err := unix.Openat(destinationFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				_ = unix.Close(sourceChild)
				return normalizePathError(err)
			}
			err = s.copyDirectoryContentsLocked(sourceChild, destinationChild, depth+1, result)
			_ = unix.Close(destinationChild)
			if err != nil {
				return err
			}
		case FileKindSymlink:
			return ErrUnsafePath
		default:
			return ErrUnsupportedType
		}
	}
	return nil
}

func (s *Session) Move(source, destination string) (result MoveResult, err error) {
	result = MoveResult{Source: source, Destination: destination}
	if err := validateStructuralPair(source, destination); err != nil {
		return result, err
	}
	if relativeDescendant(source, destination) {
		return result, ErrInvalidPath
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return result, ErrClosed
	}
	sourceParent, sourceName, err := s.openParentLocked(source)
	if err != nil {
		return result, err
	}
	if sourceParent != s.rootFD {
		defer unix.Close(sourceParent)
	}
	sourceStat, err := statAtNoFollow(sourceParent, sourceName)
	if err != nil {
		return result, err
	}
	result.Kind = fileKind(sourceStat.Mode)
	if result.Kind == FileKindSymlink {
		return result, ErrUnsafePath
	}
	if result.Kind != FileKindRegular && result.Kind != FileKindDirectory {
		return result, ErrUnsupportedType
	}
	sourceFD, err := unix.Openat(sourceParent, sourceName, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return result, normalizePathError(err)
	}
	defer unix.Close(sourceFD)
	var openedSource unix.Stat_t
	if err := unix.Fstat(sourceFD, &openedSource); err != nil {
		return result, err
	}
	if fileKind(openedSource.Mode) != result.Kind {
		return result, ErrOperationUnknown
	}
	destinationParent, destinationName, err := s.openParentLocked(destination)
	if err != nil {
		return result, err
	}
	if destinationParent != s.rootFD {
		defer unix.Close(destinationParent)
	}
	if existing, statErr := statAtNoFollow(destinationParent, destinationName); statErr == nil {
		if fileKind(existing.Mode) == FileKindSymlink {
			return result, ErrUnsafePath
		}
		return result, ErrPathExists
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return result, statErr
	}
	if err := unix.Renameat2(sourceParent, sourceName, destinationParent, destinationName, unix.RENAME_NOREPLACE); err != nil {
		if errors.Is(err, unix.EXDEV) {
			return result, ErrCrossDevice
		}
		if errors.Is(err, unix.EEXIST) {
			return result, ErrPathExists
		}
		return result, normalizePathError(err)
	}
	movedStat, statErr := statAtNoFollow(destinationParent, destinationName)
	if statErr != nil || movedStat.Dev != openedSource.Dev || movedStat.Ino != openedSource.Ino || fileKind(movedStat.Mode) != result.Kind {
		result.Status = "unknown"
		return result, ErrOperationUnknown
	}
	result.Status = "moved"
	return result, nil
}

func (s *Session) DeleteFile(relative string) (result DeleteResult, err error) {
	result = DeleteResult{Path: relative}
	if !validRelative(relative) {
		return result, relativePathError(relative)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return result, ErrClosed
	}
	parentFD, name, err := s.openParentLocked(relative)
	if err != nil {
		return result, err
	}
	if parentFD != s.rootFD {
		defer unix.Close(parentFD)
	}
	stat, err := statAtNoFollow(parentFD, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Kind = FileKindAbsent
			result.Status = "absent"
		}
		return result, err
	}
	result.Kind = fileKind(stat.Mode)
	if result.Kind == FileKindSymlink {
		return result, ErrUnsafePath
	}
	if result.Kind != FileKindRegular {
		return result, ErrNotFile
	}
	fd, err := unix.Openat(parentFD, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return result, normalizePathError(err)
	}
	defer unix.Close(fd)
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return result, err
	}
	current, err := statAtNoFollow(parentFD, name)
	if err != nil || current.Dev != opened.Dev || current.Ino != opened.Ino || fileKind(current.Mode) != FileKindRegular {
		return result, ErrOperationUnknown
	}
	if err := unix.Unlinkat(parentFD, name, 0); err != nil {
		if errors.Is(err, unix.ENOENT) {
			result.Status = "absent"
		}
		return result, normalizePathError(err)
	}
	result.Status = "deleted"
	return result, nil
}

func (s *Session) DeleteDirectory(relative string) (result DeleteResult, err error) {
	result = DeleteResult{Path: relative}
	if !validRelative(relative) {
		return result, relativePathError(relative)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return result, ErrClosed
	}
	parentFD, name, err := s.openParentLocked(relative)
	if err != nil {
		return result, err
	}
	if parentFD != s.rootFD {
		defer unix.Close(parentFD)
	}
	stat, err := statAtNoFollow(parentFD, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Kind = FileKindAbsent
			result.Status = "absent"
		}
		return result, err
	}
	result.Kind = fileKind(stat.Mode)
	if result.Kind == FileKindSymlink {
		return result, ErrUnsafePath
	}
	if result.Kind != FileKindDirectory {
		return result, ErrNotFile
	}
	directoryFD, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return result, normalizePathError(err)
	}
	directory := os.NewFile(uintptr(directoryFD), "workspace-delete-directory")
	names, readErr := directory.Readdirnames(1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return result, readErr
	}
	if closeErr != nil {
		return result, closeErr
	}
	if len(names) != 0 {
		return result, ErrDirectoryNotEmpty
	}
	current, err := statAtNoFollow(parentFD, name)
	if err != nil || current.Dev != stat.Dev || current.Ino != stat.Ino || fileKind(current.Mode) != FileKindDirectory {
		return result, ErrOperationUnknown
	}
	if err := unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR); err != nil {
		if errors.Is(err, unix.ENOTEMPTY) {
			return result, ErrDirectoryNotEmpty
		}
		return result, normalizePathError(err)
	}
	result.Status = "deleted"
	return result, nil
}

func validateStructuralPair(source, destination string) error {
	if !validRelative(source) || !validRelative(destination) || source == destination {
		if reservedRelative(source) || reservedRelative(destination) {
			return ErrReservedPath
		}
		return ErrInvalidPath
	}
	return nil
}

func relativeDescendant(parent, child string) bool {
	return strings.HasPrefix(child, parent+"/")
}

func validDirectoryEntryName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsRune(name, '/') && !strings.ContainsRune(name, '\x00')
}

func statAtNoFollow(parentFD int, name string) (unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return unix.Stat_t{}, os.ErrNotExist
		}
		return unix.Stat_t{}, normalizePathError(err)
	}
	return stat, nil
}

func removeCreatedEntryLocked(parentFD int, name string, expected unix.Stat_t) error {
	current, err := statAtNoFollow(parentFD, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if current.Dev != expected.Dev || current.Ino != expected.Ino || fileKind(current.Mode) != fileKind(expected.Mode) {
		return ErrOperationUnknown
	}
	return removeEntryLocked(parentFD, name)
}

func removeEntryLocked(parentFD int, name string) error {
	stat, err := statAtNoFollow(parentFD, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if fileKind(stat.Mode) != FileKindDirectory {
		return normalizePathError(unix.Unlinkat(parentFD, name, 0))
	}
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return normalizePathError(err)
	}
	directory := os.NewFile(uintptr(fd), "workspace-cleanup-directory")
	defer directory.Close()
	names, readErr := directory.Readdirnames(MaxStructuralEntries + 1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if len(names) > MaxStructuralEntries {
		return ErrOperationUnknown
	}
	sort.Strings(names)
	for _, child := range names {
		if !validDirectoryEntryName(child) {
			return ErrOperationUnknown
		}
		if err := removeEntryLocked(fd, child); err != nil {
			return err
		}
	}
	if err := unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR); err != nil {
		return normalizePathError(err)
	}
	return nil
}

// copyRegularEntryLocked mantém a política de cópia (arquivos 0600 e sem
// ACL/xattrs/ownership) concentrada. A cópia não promete transação contra
// escritores externos à sessão.
func (s *Session) copyRegularEntryLocked(sourceParent int, sourceName string, destinationParent int, destinationName string, result *CopyResult) error {
	sourceFD, err := unix.Openat(sourceParent, sourceName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return normalizePathError(err)
	}
	source := os.NewFile(uintptr(sourceFD), "workspace-copy-file")
	info, err := source.Stat()
	if err != nil {
		_ = source.Close()
		return err
	}
	if !info.Mode().IsRegular() {
		_ = source.Close()
		return ErrUnsupportedType
	}
	if info.Size() > MaxStructuralBytes-result.BytesCopied {
		_ = source.Close()
		return ErrStructuralLimit
	}
	destinationFD, err := unix.Openat(destinationParent, destinationName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		_ = source.Close()
		if errors.Is(err, unix.EEXIST) {
			return ErrPathExists
		}
		return normalizePathError(err)
	}
	destination := os.NewFile(uintptr(destinationFD), "workspace-copy-destination")
	allowed := MaxStructuralBytes - result.BytesCopied
	written, copyErr := io.CopyN(destination, source, allowed+1)
	if copyErr != nil && !errors.Is(copyErr, io.EOF) {
		_ = destination.Close()
		_ = source.Close()
		return copyErr
	}
	if written > allowed {
		_ = destination.Close()
		_ = source.Close()
		return ErrStructuralLimit
	}
	if written != info.Size() {
		_ = destination.Close()
		_ = source.Close()
		return ErrOperationUnknown
	}
	if err := destination.Sync(); err != nil {
		_ = destination.Close()
		_ = source.Close()
		return err
	}
	destinationCloseErr := destination.Close()
	sourceCloseErr := source.Close()
	if destinationCloseErr != nil {
		return destinationCloseErr
	}
	if sourceCloseErr != nil {
		return sourceCloseErr
	}
	result.EntriesCopied++
	result.FilesCopied++
	result.BytesCopied += written
	return nil
}

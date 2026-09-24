package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const (
	MaxRelativePathBytes  = 4096
	MaxFindResults        = 256
	MaxFindDepth          = 64
	MaxFindVisited        = 4096
	MaxSearchFiles        = 256
	MaxSearchMatches      = 256
	MaxSearchBytes        = 8 << 20
	MaxSearchFileBytes    = 1 << 20
	MaxSearchSnippetBytes = 512
	MaxSearchLineBytes    = 4096
	MaxPatternBytes       = 256
	MaxQueryBytes         = 1024
)

var (
	ErrInvalidPattern = errors.New("invalid workspace search pattern")
	ErrInvalidQuery   = errors.New("invalid workspace search query")
	ErrInvalidHash    = errors.New("invalid workspace content hash")
	ErrInvalidLimit   = errors.New("invalid workspace operation limit")
	ErrPathExists     = errors.New("workspace path already exists")
)

type FileKind string

const (
	FileKindRegular   FileKind = "regular_file"
	FileKindDirectory FileKind = "directory"
	FileKindSymlink   FileKind = "symlink"
	FileKindOther     FileKind = "other"
	FileKindAbsent    FileKind = "absent"
)

type PathStat struct {
	Path  string   `json:"path"`
	Kind  FileKind `json:"kind"`
	Found bool     `json:"found"`
	Size  int64    `json:"size,omitempty"`
}

type FindMatch struct {
	Path string   `json:"path"`
	Kind FileKind `json:"kind"`
}

type FindResult struct {
	Root      string      `json:"root"`
	Pattern   string      `json:"pattern"`
	Matches   []FindMatch `json:"matches"`
	Visited   int         `json:"visited"`
	Truncated bool        `json:"truncated"`
}

type SearchMatch struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Snippet   string `json:"snippet"`
	Truncated bool   `json:"truncated"`
}

type SearchResult struct {
	Root          string        `json:"root"`
	Query         string        `json:"query"`
	Matches       []SearchMatch `json:"matches"`
	FilesSearched int           `json:"files_searched"`
	FilesSkipped  int           `json:"files_skipped"`
	BytesRead     int64         `json:"bytes_read"`
	Truncated     bool          `json:"truncated"`
}

type DirectoryResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type TextFileResult struct {
	Path           string `json:"path"`
	Status         string `json:"status"`
	PreviousSHA256 string `json:"previous_sha256,omitempty"`
	SHA256         string `json:"sha256"`
}

func (s *Session) StatPath(relative string) (PathStat, error) {
	if relative != "." && !validRelative(relative) {
		return PathStat{}, relativePathError(relative)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return PathStat{}, ErrClosed
	}
	return s.statPathLocked(relative)
}

func (s *Session) statPathLocked(relative string) (PathStat, error) {
	if relative == "." {
		var stat unix.Stat_t
		if err := unix.Fstat(s.rootFD, &stat); err != nil {
			return PathStat{}, err
		}
		return PathStat{Path: relative, Kind: FileKindDirectory, Found: true}, nil
	}
	parentFD, name, err := s.openParentLocked(relative)
	if err != nil {
		return PathStat{}, err
	}
	if parentFD != s.rootFD {
		defer unix.Close(parentFD)
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return PathStat{Path: relative, Kind: FileKindAbsent, Found: false}, nil
		}
		return PathStat{}, normalizePathError(err)
	}
	result := PathStat{Path: relative, Kind: fileKind(stat.Mode), Found: true}
	if result.Kind == FileKindRegular {
		result.Size = stat.Size
	}
	return result, nil
}

func fileKind(mode uint32) FileKind {
	switch mode & syscall.S_IFMT {
	case syscall.S_IFREG:
		return FileKindRegular
	case syscall.S_IFDIR:
		return FileKindDirectory
	case syscall.S_IFLNK:
		return FileKindSymlink
	default:
		return FileKindOther
	}
}

func (s *Session) FindPaths(root, pattern string, maxResults, maxDepth int) (FindResult, error) {
	if root == "" {
		root = "."
	}
	if root != "." && !validRelative(root) {
		return FindResult{}, relativePathError(root)
	}
	if len(pattern) == 0 || len(pattern) > MaxPatternBytes || !utf8.ValidString(pattern) || strings.ContainsRune(pattern, '\x00') || strings.HasPrefix(pattern, "/") {
		return FindResult{}, ErrInvalidPattern
	}
	if _, err := path.Match(pattern, "probe"); err != nil {
		return FindResult{}, ErrInvalidPattern
	}
	maxResults, maxDepth, err := normalizeFindLimits(maxResults, maxDepth)
	if err != nil {
		return FindResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return FindResult{}, ErrClosed
	}
	result := FindResult{Root: root, Pattern: pattern, Matches: make([]FindMatch, 0)}
	result.Visited, result.Truncated, err = s.walkLocked(root, maxDepth, func(relative string, kind FileKind, _ unix.Stat_t) (bool, error) {
		if !matchesFindPattern(pattern, relative) {
			return false, nil
		}
		if len(result.Matches) >= maxResults {
			return true, nil
		}
		result.Matches = append(result.Matches, FindMatch{Path: relative, Kind: kind})
		return len(result.Matches) >= maxResults, nil
	})
	return result, err
}

func normalizeFindLimits(maxResults, maxDepth int) (int, int, error) {
	if maxResults == 0 {
		maxResults = MaxFindResults
	}
	if maxDepth == 0 {
		maxDepth = MaxFindDepth
	}
	if maxResults < 1 || maxResults > MaxFindResults || maxDepth < 1 || maxDepth > MaxFindDepth {
		return 0, 0, ErrInvalidLimit
	}
	return maxResults, maxDepth, nil
}

func matchesFindPattern(pattern, relative string) bool {
	if matched, _ := path.Match(pattern, relative); matched {
		return true
	}
	matched, _ := path.Match(pattern, path.Base(relative))
	return matched
}

func (s *Session) SearchText(root, query string, maxResults int) (SearchResult, error) {
	if root == "" {
		root = "."
	}
	if root != "." && !validRelative(root) {
		return SearchResult{}, relativePathError(root)
	}
	if len(query) == 0 || len(query) > MaxQueryBytes || !utf8.ValidString(query) || strings.ContainsRune(query, '\x00') || strings.ContainsRune(query, '\n') || strings.ContainsRune(query, '\r') {
		return SearchResult{}, ErrInvalidQuery
	}
	if maxResults == 0 {
		maxResults = MaxSearchMatches
	}
	if maxResults < 1 || maxResults > MaxSearchMatches {
		return SearchResult{}, ErrInvalidLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return SearchResult{}, ErrClosed
	}
	result := SearchResult{Root: root, Query: query, Matches: make([]SearchMatch, 0)}
	var err error
	_, result.Truncated, err = s.walkLocked(root, MaxFindDepth, func(relative string, kind FileKind, stat unix.Stat_t) (bool, error) {
		if kind != FileKindRegular {
			return false, nil
		}
		if result.FilesSearched >= MaxSearchFiles || result.BytesRead >= MaxSearchBytes {
			return true, nil
		}
		if stat.Size > MaxSearchFileBytes || stat.Size > MaxSearchBytes-result.BytesRead {
			result.FilesSkipped++
			if stat.Size > MaxSearchBytes-result.BytesRead {
				return true, nil
			}
			return false, nil
		}
		data, err := s.readRegularFileLocked(relative, MaxSearchFileBytes)
		if err != nil {
			if errors.Is(err, ErrNotText) || errors.Is(err, ErrTooLarge) || errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrUnsafePath) {
				result.FilesSkipped++
				return false, nil
			}
			return false, err
		}
		result.FilesSearched++
		result.BytesRead += int64(len(data))
		matches, stop := searchTextLines(relative, string(data), query, maxResults-len(result.Matches))
		result.Matches = append(result.Matches, matches...)
		return stop, nil
	})
	if len(result.Matches) >= maxResults {
		result.Truncated = true
	}
	sort.Slice(result.Matches, func(i, j int) bool {
		if result.Matches[i].Path != result.Matches[j].Path {
			return result.Matches[i].Path < result.Matches[j].Path
		}
		if result.Matches[i].Line != result.Matches[j].Line {
			return result.Matches[i].Line < result.Matches[j].Line
		}
		return result.Matches[i].Column < result.Matches[j].Column
	})
	return result, err
}

func searchTextLines(relative, text, query string, limit int) ([]SearchMatch, bool) {
	matches := make([]SearchMatch, 0)
	lineStart, lineNumber := 0, 1
	for lineStart <= len(text) {
		lineEnd := strings.IndexByte(text[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(text)
		} else {
			lineEnd += lineStart
		}
		line := text[lineStart:lineEnd]
		for offset := 0; offset <= len(line)-len(query); {
			matchOffset := strings.Index(line[offset:], query)
			if matchOffset < 0 {
				break
			}
			matchOffset += offset
			snippet, truncated := searchSnippet(line, matchOffset, len(query))
			if len(line) > MaxSearchLineBytes {
				truncated = true
			}
			matches = append(matches, SearchMatch{Path: relative, Line: lineNumber, Column: matchOffset + 1, Snippet: snippet, Truncated: truncated})
			if len(matches) >= limit {
				return matches, true
			}
			offset = matchOffset + 1
		}
		if lineEnd == len(text) {
			break
		}
		lineStart = lineEnd + 1
		lineNumber++
	}
	return matches, false
}

func searchSnippet(line string, matchOffset, matchLength int) (string, bool) {
	if len(line) <= MaxSearchSnippetBytes {
		return line, false
	}
	start := matchOffset - MaxSearchSnippetBytes/4
	if start < 0 {
		start = 0
	}
	end := start + MaxSearchSnippetBytes
	if end > len(line) {
		end = len(line)
		start = end - MaxSearchSnippetBytes
	}
	if start > matchOffset {
		start = matchOffset
	}
	if end < matchOffset+matchLength {
		end = matchOffset + matchLength
		if end > len(line) {
			end = len(line)
		}
		start = end - MaxSearchSnippetBytes
		if start < 0 {
			start = 0
		}
	}
	return line[start:end], true
}

func (s *Session) CreateDirectory(relative string) (DirectoryResult, error) {
	if !validRelative(relative) {
		return DirectoryResult{}, relativePathError(relative)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return DirectoryResult{}, ErrClosed
	}
	parentFD, name, err := s.openParentLocked(relative)
	if err != nil {
		return DirectoryResult{}, err
	}
	if parentFD != s.rootFD {
		defer unix.Close(parentFD)
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		if fileKind(stat.Mode) == FileKindDirectory {
			return DirectoryResult{Path: relative, Status: "already_exists"}, nil
		}
		if fileKind(stat.Mode) == FileKindSymlink {
			return DirectoryResult{}, ErrUnsafePath
		}
		return DirectoryResult{}, ErrPathExists
	} else if !errors.Is(err, unix.ENOENT) {
		return DirectoryResult{}, normalizePathError(err)
	}
	if err := unix.Mkdirat(parentFD, name, 0700); err != nil {
		if errors.Is(err, unix.EEXIST) {
			if statErr := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); statErr == nil && fileKind(stat.Mode) == FileKindDirectory {
				return DirectoryResult{Path: relative, Status: "already_exists"}, nil
			}
		}
		return DirectoryResult{}, normalizePathError(err)
	}
	return DirectoryResult{Path: relative, Status: "created"}, nil
}

func (s *Session) CreateTextFile(relative, content string) (TextFileResult, error) {
	if !validRelative(relative) {
		return TextFileResult{}, relativePathError(relative)
	}
	if !validTextContent(content) {
		return TextFileResult{}, ErrNotText
	}
	if len(content) > MaxTextBytes {
		return TextFileResult{}, ErrTooLarge
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return TextFileResult{}, ErrClosed
	}
	parentFD, name, err := s.openParentLocked(relative)
	if err != nil {
		return TextFileResult{}, err
	}
	if parentFD != s.rootFD {
		defer unix.Close(parentFD)
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		if fileKind(stat.Mode) == FileKindSymlink {
			return TextFileResult{}, ErrUnsafePath
		}
		return TextFileResult{}, ErrPathExists
	} else if !errors.Is(err, unix.ENOENT) {
		return TextFileResult{}, normalizePathError(err)
	}
	if err := s.publishNewFileLocked(parentFD, name, []byte(content)); err != nil {
		return TextFileResult{}, err
	}
	return TextFileResult{Path: relative, Status: "created", SHA256: sha256Text(content)}, nil
}

func (s *Session) WriteTextFile(relative, expectedSHA256, content string) (TextFileResult, error) {
	if !validRelative(relative) {
		return TextFileResult{}, relativePathError(relative)
	}
	if !validTextContent(content) {
		return TextFileResult{}, ErrNotText
	}
	if len(content) > MaxTextBytes {
		return TextFileResult{}, ErrTooLarge
	}
	expected, err := decodeSHA256(expectedSHA256)
	if err != nil {
		return TextFileResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return TextFileResult{}, ErrClosed
	}
	parentFD, name, err := s.openParentLocked(relative)
	if err != nil {
		return TextFileResult{}, err
	}
	if parentFD != s.rootFD {
		defer unix.Close(parentFD)
	}
	currentFD, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return TextFileResult{}, normalizePathError(err)
	}
	current := os.NewFile(uintptr(currentFD), "workspace-current")
	info, err := current.Stat()
	if err != nil {
		_ = current.Close()
		return TextFileResult{}, err
	}
	if !info.Mode().IsRegular() {
		_ = current.Close()
		return TextFileResult{}, ErrNotFile
	}
	oldContent, err := readTextFile(current, MaxTextBytes)
	closeErr := current.Close()
	if err != nil {
		return TextFileResult{}, err
	}
	if closeErr != nil {
		return TextFileResult{}, closeErr
	}
	oldHash := sha256.Sum256([]byte(oldContent))
	if !equalSHA256(oldHash, expected) {
		return TextFileResult{}, ErrConflict
	}
	if err := s.publishReplacementLocked(parentFD, name, []byte(content), info.Mode().Perm(), oldHash); err != nil {
		return TextFileResult{}, err
	}
	return TextFileResult{Path: relative, Status: "updated", PreviousSHA256: hex.EncodeToString(oldHash[:]), SHA256: sha256Text(content)}, nil
}

func decodeSHA256(raw string) ([32]byte, error) {
	var result [32]byte
	if len(raw) != sha256.Size*2 {
		return result, ErrInvalidHash
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return result, ErrInvalidHash
	}
	copy(result[:], decoded)
	return result, nil
}

func equalSHA256(left, right [32]byte) bool {
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sha256Text(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func (s *Session) openParentLocked(relative string) (int, string, error) {
	parts := strings.Split(relative, "/")
	parentFD := s.rootFD
	for _, part := range parts[:len(parts)-1] {
		next, err := syscall.Openat(parentFD, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if parentFD != s.rootFD {
			_ = syscall.Close(parentFD)
		}
		if err != nil {
			return -1, "", normalizePathError(err)
		}
		parentFD = next
	}
	return parentFD, parts[len(parts)-1], nil
}

func (s *Session) openDirectoryLocked(relative string) (int, error) {
	if relative == "." {
		fd, err := syscall.Openat(s.rootFD, ".", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if err != nil {
			return -1, normalizePathError(err)
		}
		return fd, nil
	}
	fd := s.rootFD
	for _, part := range strings.Split(relative, "/") {
		next, err := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if fd != s.rootFD {
			_ = syscall.Close(fd)
		}
		if err != nil {
			return -1, normalizePathError(err)
		}
		fd = next
	}
	return fd, nil
}

func (s *Session) readRegularFileLocked(relative string, limit int64) ([]byte, error) {
	parentFD, name, err := s.openParentLocked(relative)
	if err != nil {
		return nil, err
	}
	if parentFD != s.rootFD {
		defer unix.Close(parentFD)
	}
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, normalizePathError(err)
	}
	file := os.NewFile(uintptr(fd), "workspace-search-file")
	defer file.Close()
	return readTextFile(file, limit)
}

func readTextFile(file *os.File, limit int64) ([]byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNotFile
	}
	if info.Size() > limit {
		return nil, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrTooLarge
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
		return nil, ErrNotText
	}
	return data, nil
}

func (s *Session) publishNewFileLocked(parentFD int, name string, content []byte) error {
	tempName, err := newEditTempName()
	if err != nil {
		return err
	}
	tempFD, err := unix.Openat(parentFD, tempName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	temp := os.NewFile(uintptr(tempFD), "workspace-create-temp")
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = unix.Unlinkat(parentFD, tempName, 0)
		}
	}()
	if err := writeAll(temp, content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := unix.Renameat2(parentFD, tempName, parentFD, name, unix.RENAME_NOREPLACE); err != nil {
		return normalizePathError(err)
	}
	removeTemp = false
	return nil
}

func (s *Session) publishReplacementLocked(parentFD int, name string, content []byte, mode os.FileMode, expected [32]byte) error {
	currentFD, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return normalizePathError(err)
	}
	current := os.NewFile(uintptr(currentFD), "workspace-current-check")
	currentInfo, statErr := current.Stat()
	if statErr != nil {
		_ = current.Close()
		return statErr
	}
	currentContent, readErr := readTextFile(current, MaxTextBytes)
	closeErr := current.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	currentHash := sha256.Sum256(currentContent)
	if !equalSHA256(currentHash, expected) || currentInfo.Mode().Perm() != mode.Perm() {
		return ErrConflict
	}
	tempName, err := newEditTempName()
	if err != nil {
		return err
	}
	tempFD, err := unix.Openat(parentFD, tempName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, uint32(mode.Perm()))
	if err != nil {
		return err
	}
	temp := os.NewFile(uintptr(tempFD), "workspace-write-temp")
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = unix.Unlinkat(parentFD, tempName, 0)
		}
	}()
	if err := writeAll(temp, content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := unix.Renameat(parentFD, tempName, parentFD, name); err != nil {
		return normalizePathError(err)
	}
	removeTemp = false
	return nil
}

type walkVisitor func(relative string, kind FileKind, stat unix.Stat_t) (stop bool, err error)

func (s *Session) walkLocked(root string, maxDepth int, visit walkVisitor) (int, bool, error) {
	rootFD, err := s.openDirectoryLocked(root)
	if err != nil {
		return 0, false, err
	}
	defer unix.Close(rootFD)
	visited, truncated, err := s.walkDirectoryLocked(rootFD, root, 0, maxDepth, 0, visit)
	return visited, truncated, err
}

func (s *Session) walkDirectoryLocked(dirFD int, relativeRoot string, depth, maxDepth, visited int, visit walkVisitor) (int, bool, error) {
	directory := os.NewFile(uintptr(dirFD), "workspace-walk-directory")
	defer directory.Close()
	names, err := directory.Readdirnames(MaxFindVisited + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return visited, false, err
	}
	sort.Strings(names)
	for _, name := range names {
		if !utf8.ValidString(name) || name == "." || name == ".." || strings.ContainsRune(name, '\x00') {
			return visited, false, ErrInvalidPath
		}
		if name == ".git" {
			continue
		}
		if visited >= MaxFindVisited {
			return visited, true, nil
		}
		visited++
		relative := name
		if relativeRoot != "." {
			relative = relativeRoot + "/" + name
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(dirFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			return visited, false, normalizePathError(err)
		}
		kind := fileKind(stat.Mode)
		stop, err := visit(relative, kind, stat)
		if err != nil {
			return visited, false, err
		}
		if stop {
			return visited, true, nil
		}
		if kind != FileKindDirectory {
			continue
		}
		if depth+1 >= maxDepth {
			return visited, true, nil
		}
		childFD, err := syscall.Openat(dirFD, name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if err != nil {
			if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.ENOTDIR) {
				continue
			}
			return visited, false, err
		}
		visited, truncated, err := s.walkDirectoryLocked(childFD, relative, depth+1, maxDepth, visited, visit)
		if err != nil || truncated {
			return visited, truncated, err
		}
	}
	return visited, false, nil
}

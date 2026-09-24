package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxGitIndexPaths     = 128
	MaxGitIndexPathBytes = 32 << 10
	maxGitIndexOutput    = 8 << 20
	maxGitFileHashBytes  = 256 << 20
)

var (
	ErrManagedWorktreeRequired = errors.New("managed worktree is required")
	ErrGitIndexInput           = errors.New("invalid Git index input")
	ErrGitIndexStale           = errors.New("Git index precondition is stale")
	ErrGitWorktreeStale        = errors.New("Git working tree precondition is stale")
	ErrGitIndexConflict        = errors.New("Git index contains an unsupported conflict")
	ErrUnsupportedGitPath      = errors.New("Git path is unsupported")
	ErrUnsupportedGitFilter    = errors.New("Git executable filters are unsupported")
)

// GitIndexStatusEntry é a representação segura para a UI. Path é sempre
// relativo à raiz aprovada; nenhum conteúdo, raiz absoluta ou metadado .git é
// retornado.
type GitIndexStatusEntry struct {
	Path          string `json:"path"`
	Tracked       bool   `json:"tracked"`
	Untracked     bool   `json:"untracked"`
	Staged        bool   `json:"staged"`
	Unstaged      bool   `json:"unstaged"`
	Deleted       bool   `json:"deleted"`
	Modified      bool   `json:"modified"`
	Added         bool   `json:"added"`
	Renamed       bool   `json:"renamed"`
	Conflict      bool   `json:"conflict"`
	IndexObjectID string `json:"index_oid,omitempty"`
}

type GitIndexStatus struct {
	Entries     []GitIndexStatusEntry `json:"entries"`
	IndexSHA256 string                `json:"index_sha256"`
}

type GitIndexEntry struct {
	Path             string `json:"path"`
	ExpectedSHA256   string `json:"expected_sha256,omitempty"`
	ExpectedIndexOID string `json:"expected_index_oid,omitempty"`
}

type GitIndexMutationResult struct {
	Status         string   `json:"status"`
	RequestedPaths []string `json:"requested_paths"`
	ChangedPaths   []string `json:"changed_paths"`
	OldIndexSHA256 string   `json:"old_index_sha256"`
	NewIndexSHA256 string   `json:"new_index_sha256"`
	Partial        bool     `json:"partial_or_unknown"`
	ErrorCode      string   `json:"error_code,omitempty"`
}

func CaptureGitIndexStatus(session ProcessDirectory) (GitIndexStatus, error) {
	if session == nil {
		return GitIndexStatus{}, ErrGitIndexInput
	}
	var result GitIndexStatus
	err := session.WithProcessDir(func(directory string) error {
		return captureGitIndexStatus(directory, &result)
	})
	return result, err
}

func captureGitIndexStatus(directory string, result *GitIndexStatus) error {
	runner := managedGitRunner{hooksPath: os.DevNull}
	statusBytes, statusTruncated, err := runner.captureOutput(directory, maxGitIndexOutput, "status", "--porcelain=v2", "-z", "--untracked-files=all", "--")
	if err != nil {
		return ErrManagedGit
	}
	if statusTruncated {
		return ErrTooLarge
	}
	indexBytes, indexTruncated, err := runner.captureOutput(directory, maxGitIndexOutput, "ls-files", "--stage", "-z", "--")
	if err != nil {
		return ErrManagedGit
	}
	if indexTruncated {
		return ErrTooLarge
	}
	entries, err := parseGitStatus(statusBytes)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(indexBytes)
	result.Entries = entries
	result.IndexSHA256 = hex.EncodeToString(hash[:])
	return nil
}

func StageGitPaths(session ProcessDirectory, expectedIndexSHA256 string, entries []GitIndexEntry) (GitIndexMutationResult, error) {
	return mutateGitPaths(session, expectedIndexSHA256, entries, true)
}

func UnstageGitPaths(session ProcessDirectory, expectedIndexSHA256 string, entries []GitIndexEntry) (GitIndexMutationResult, error) {
	return mutateGitPaths(session, expectedIndexSHA256, entries, false)
}

func mutateGitPaths(session ProcessDirectory, expectedIndexSHA256 string, entries []GitIndexEntry, stage bool) (GitIndexMutationResult, error) {
	paths, err := validateGitIndexRequest(expectedIndexSHA256, entries)
	if err != nil {
		return GitIndexMutationResult{}, err
	}
	result := GitIndexMutationResult{RequestedPaths: append([]string(nil), paths...)}
	if session == nil {
		return result, ErrGitIndexInput
	}
	err = session.WithProcessDir(func(directory string) error {
		before, err := readGitIndexStatus(directory)
		if err != nil {
			return err
		}
		result.OldIndexSHA256 = before.IndexSHA256
		if before.IndexSHA256 != expectedIndexSHA256 {
			result.Status = "failed_no_change"
			result.ErrorCode = "stale_index_hash"
			return ErrGitIndexStale
		}
		if err := validateGitIndexEntries(directory, before, entries, stage); err != nil {
			return err
		}
		runner := managedGitRunner{hooksPath: os.DevNull}
		if stage {
			// A filter can be introduced after worktree creation; revalidate in
			// the same serialized grant immediately before git add.
			if err := ensureNoExecutableFilters(runner, directory); err != nil {
				return err
			}
		}
		input := []byte(strings.Join(paths, "\x00") + "\x00")
		var gitErr error
		if stage {
			gitErr = runner.runInput(directory, input, "--literal-pathspecs", "add", "--pathspec-from-file=-", "--pathspec-file-nul")
		} else {
			gitErr = runner.runInput(directory, input, "--literal-pathspecs", "restore", "--staged", "--source=HEAD", "--pathspec-from-file=-", "--pathspec-file-nul")
		}
		after, statusErr := readGitIndexStatus(directory)
		if statusErr != nil {
			result.Status = "partial_or_unknown"
			result.Partial = true
			result.ErrorCode = "postcondition_unknown"
			return statusErr
		}
		result.NewIndexSHA256 = after.IndexSHA256
		if gitErr != nil {
			result.Partial = after.IndexSHA256 != before.IndexSHA256
			if result.Partial {
				result.Status = "partial_or_unknown"
				result.ErrorCode = "git_error_after_change"
			} else {
				result.Status = "failed_no_change"
				result.ErrorCode = "git_error_no_change"
			}
			return nil
		}
		result.Status = "staged"
		if !stage {
			result.Status = "unstaged"
		}
		result.ChangedPaths = changedGitPaths(before, after, paths)
		return nil
	})
	return result, err
}

func readGitIndexStatus(directory string) (GitIndexStatus, error) {
	var result GitIndexStatus
	if err := captureGitIndexStatus(directory, &result); err != nil {
		return GitIndexStatus{}, err
	}
	return result, nil
}

func validateGitIndexRequest(expected string, entries []GitIndexEntry) ([]string, error) {
	if len(entries) == 0 || len(entries) > MaxGitIndexPaths || len(expected) != sha256.Size*2 {
		return nil, ErrGitIndexInput
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return nil, ErrGitIndexInput
	}
	paths := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	total := 0
	for _, entry := range entries {
		if !validRelative(entry.Path) || strings.ContainsRune(entry.Path, '\x00') || entry.Path == "." || filepath.IsAbs(entry.Path) {
			return nil, ErrGitIndexInput
		}
		if _, ok := seen[entry.Path]; ok {
			return nil, ErrGitIndexInput
		}
		seen[entry.Path] = struct{}{}
		total += len(entry.Path)
		if total > MaxGitIndexPathBytes {
			return nil, ErrGitIndexInput
		}
		paths = append(paths, entry.Path)
	}
	return paths, nil
}

func validateGitIndexEntries(directory string, status GitIndexStatus, requested []GitIndexEntry, stage bool) error {
	byPath := make(map[string]GitIndexStatusEntry, len(status.Entries))
	for _, entry := range status.Entries {
		byPath[entry.Path] = entry
	}
	for _, request := range requested {
		entry, ok := byPath[request.Path]
		if !ok {
			return ErrGitWorktreeStale
		}
		if entry.Conflict {
			return ErrGitIndexConflict
		}
		if stage {
			if err := validatePathComponents(directory, request.Path); err != nil {
				return err
			}
			info, err := os.Lstat(filepath.Join(directory, filepath.FromSlash(request.Path)))
			if err != nil {
				if !entry.Deleted || request.ExpectedIndexOID == "" || request.ExpectedIndexOID != entry.IndexObjectID {
					return ErrGitWorktreeStale
				}
				continue
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return ErrUnsupportedGitPath
			}
			if request.ExpectedSHA256 == "" {
				return ErrGitWorktreeStale
			}
			actual, err := hashRegularFile(filepath.Join(directory, filepath.FromSlash(request.Path)), info.Size())
			if err != nil || !strings.EqualFold(actual, request.ExpectedSHA256) {
				return ErrGitWorktreeStale
			}
		} else {
			if entry.Untracked && !entry.Staged {
				return ErrGitWorktreeStale
			}
			if entry.IndexObjectID == "" && request.ExpectedIndexOID != "" {
				return ErrGitWorktreeStale
			}
			if request.ExpectedIndexOID != "" && request.ExpectedIndexOID != entry.IndexObjectID {
				return ErrGitWorktreeStale
			}
		}
	}
	return nil
}

func ensureNoExecutableFilters(runner managedGitRunner, directory string) error {
	output, _, err := runner.captureOutput(directory, managedGitOutput, "config", "--local", "--get-regexp", `^filter\..*\.(clean|smudge|process)$`)
	if err == nil && len(strings.TrimSpace(string(output))) == 0 {
		return nil
	}
	if code, ok := managedGitExitCode(err); ok && code == 1 && len(output) == 0 {
		return nil
	}
	return ErrUnsupportedGitFilter
}

func validatePathComponents(directory, relative string) error {
	current := directory
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err != nil {
			if index == len(parts)-1 && errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return ErrGitWorktreeStale
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsupportedGitPath
		}
		if index < len(parts)-1 && !info.IsDir() {
			return ErrGitWorktreeStale
		}
	}
	return nil
}

func hashRegularFile(path string, size int64) (string, error) {
	if size < 0 || size > maxGitFileHashBytes {
		return "", ErrTooLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.CopyN(hash, file, size); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func changedGitPaths(before, after GitIndexStatus, paths []string) []string {
	beforeEntries := make(map[string]GitIndexStatusEntry, len(before.Entries))
	afterEntries := make(map[string]GitIndexStatusEntry, len(after.Entries))
	for _, entry := range before.Entries {
		beforeEntries[entry.Path] = entry
	}
	for _, entry := range after.Entries {
		afterEntries[entry.Path] = entry
	}
	changed := make([]string, 0, len(paths))
	for _, path := range paths {
		if fmt.Sprint(beforeEntries[path]) != fmt.Sprint(afterEntries[path]) {
			changed = append(changed, path)
		}
	}
	return changed
}

func parseGitStatus(data []byte) ([]GitIndexStatusEntry, error) {
	parts := bytesSplitNUL(data)
	entries := make([]GitIndexStatusEntry, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		record := string(parts[i])
		if record == "" {
			continue
		}
		if len(record) < 3 {
			return nil, ErrManagedGit
		}
		kind := record[0]
		if kind == '!' {
			continue
		}
		if kind == '?' {
			path := strings.TrimPrefix(record, "? ")
			if !validRelative(path) {
				return nil, ErrManagedGit
			}
			entries = append(entries, GitIndexStatusEntry{Path: path, Untracked: true})
			continue
		}
		fieldCount := 9
		if kind == '2' {
			fieldCount = 10
		}
		if kind == 'u' {
			fieldCount = 11
		}
		fields := strings.SplitN(record, " ", fieldCount)
		if kind == '2' {
			if i+1 >= len(parts) {
				return nil, ErrManagedGit
			}
			i++
		}
		if len(fields) < fieldCount || (kind != '1' && kind != '2' && kind != 'u') {
			return nil, ErrManagedGit
		}
		xy := fields[1]
		if len(xy) != 2 || strings.HasPrefix(fields[2], "S") {
			return nil, ErrUnsupportedGitPath
		}
		path := fields[fieldCount-1]
		if !validRelative(path) {
			return nil, ErrManagedGit
		}
		entry := GitIndexStatusEntry{Path: path, Tracked: true, Staged: xy[0] != '.', Unstaged: xy[1] != '.', Conflict: kind == 'u'}
		entry.Deleted = strings.ContainsRune(xy, 'D')
		entry.Modified = strings.ContainsRune(xy, 'M')
		entry.Added = strings.ContainsRune(xy, 'A')
		entry.Renamed = strings.ContainsRune(xy, 'R') || strings.ContainsRune(xy, 'C')
		if kind != 'u' && len(fields) > 7 {
			entry.IndexObjectID = fields[7]
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func bytesSplitNUL(data []byte) [][]byte {
	parts := make([][]byte, 0, 16)
	start := 0
	for index, value := range data {
		if value == 0 {
			parts = append(parts, data[start:index])
			start = index + 1
		}
	}
	if start < len(data) {
		parts = append(parts, data[start:])
	}
	return parts
}

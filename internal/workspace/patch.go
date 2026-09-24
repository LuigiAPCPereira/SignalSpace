package workspace

import (
	"errors"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	MaxPatchOperations   = 128
	MaxPatchContentBytes = 1 << 20
	PatchCreateFile      = "create_file"
	PatchWriteFile       = "write_file"
	PatchMovePath        = "move_path"
	PatchDeleteFile      = "delete_file"
	PatchCreateDirectory = "create_directory"
)

var (
	ErrPatchInvalid       = errors.New("invalid workspace patch")
	ErrPatchConflict      = errors.New("workspace patch has conflicting operations")
	ErrPatchHashConflict  = errors.New("workspace patch hash precondition failed")
	ErrPatchNotFound      = errors.New("workspace patch path not found")
	ErrPatchAlreadyExists = errors.New("workspace patch destination already exists")
)

// PatchOperation é uma operação fechada e tipada. O parser MCP rejeita
// campos extras antes de chegar a esta camada; a workspace ainda valida
// limites, caminhos e precondições para não depender do transporte.
type PatchOperation struct {
	Type           string
	Path           string
	Source         string
	Destination    string
	Content        string
	ExpectedSHA256 string
}

type PatchOperationResult struct {
	Index          int    `json:"index"`
	Type           string `json:"type"`
	Status         string `json:"status"`
	Path           string `json:"path,omitempty"`
	Source         string `json:"source,omitempty"`
	Destination    string `json:"destination,omitempty"`
	PreviousSHA256 string `json:"previous_sha256,omitempty"`
	SHA256         string `json:"sha256,omitempty"`
	Error          string `json:"error,omitempty"`
}

type PatchSummary struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Moved   int `json:"moved"`
	Deleted int `json:"deleted"`
}

type PatchResult struct {
	Status              string                 `json:"status"`
	OperationsRequested int                    `json:"operations_requested"`
	OperationsApplied   int                    `json:"operations_applied"`
	Operations          []PatchOperationResult `json:"operations"`
	Conflict            string                 `json:"conflict,omitempty"`
	Partial             bool                   `json:"partial,omitempty"`
	Rollback            string                 `json:"rollback,omitempty"`
	Summary             PatchSummary           `json:"summary"`
}

type patchUndo struct {
	operation     PatchOperation
	oldContent    string
	oldSHA256     string
	directoryNoOp bool
}

// ApplyPatch executa uma sequência estruturada com preflight completo antes
// de qualquer mutação. A operação não promete transação de filesystem: se uma
// mutação falhar, desfaz as anteriores por compensação e informa o resultado.
func (s *Session) ApplyPatch(operations []PatchOperation) (PatchResult, error) {
	result := PatchResult{
		Status:              "not_started",
		OperationsRequested: len(operations),
		Operations:          make([]PatchOperationResult, 0, len(operations)),
	}
	if err := validatePatchOperations(operations); err != nil {
		result.Status = patchStatus(err)
		result.Conflict = patchErrorCode(err)
		result.Rollback = "not_started"
		return result, err
	}

	undo, err := s.preflightPatch(operations)
	if err != nil {
		result.Status = patchStatus(err)
		result.Conflict = patchErrorCode(err)
		result.Rollback = "not_started"
		for index, operation := range operations {
			item := patchOperationResult(index, operation)
			item.Status = "not_started"
			result.Operations = append(result.Operations, item)
		}
		return result, err
	}

	applied := make([]int, 0, len(operations))
	for index, operation := range operations {
		item := patchOperationResult(index, operation)
		var err error
		mutated := true
		switch operation.Type {
		case PatchCreateFile:
			var created TextFileResult
			created, err = s.CreateTextFile(operation.Path, operation.Content)
			item.Status = created.Status
			item.Path = created.Path
			item.SHA256 = created.SHA256
		case PatchWriteFile:
			var updated TextFileResult
			updated, err = s.WriteTextFile(operation.Path, operation.ExpectedSHA256, operation.Content)
			item.Status = updated.Status
			item.Path = updated.Path
			item.PreviousSHA256 = updated.PreviousSHA256
			item.SHA256 = updated.SHA256
		case PatchMovePath:
			var moved MoveResult
			moved, err = s.Move(operation.Source, operation.Destination)
			item.Status = moved.Status
			item.Source = moved.Source
			item.Destination = moved.Destination
		case PatchDeleteFile:
			var deleted DeleteResult
			deleted, err = s.deleteFileExpected(operation.Path, operation.ExpectedSHA256)
			item.Status = deleted.Status
			item.Path = deleted.Path
			item.PreviousSHA256 = undo[index].oldSHA256
		case PatchCreateDirectory:
			var created DirectoryResult
			created, err = s.CreateDirectory(operation.Path)
			item.Status = created.Status
			item.Path = created.Path
			mutated = created.Status == "created"
		default:
			err = ErrPatchInvalid
		}
		if err != nil {
			item.Status = patchStatus(err)
			item.Error = patchErrorCode(err)
			result.Operations = append(result.Operations, item)
			for remaining := index + 1; remaining < len(operations); remaining++ {
				result.Operations = append(result.Operations, patchOperationResult(remaining, operations[remaining]))
			}
			if len(applied) == 0 {
				result.Status = patchStatus(err)
				result.Rollback = "not_started"
				return result, err
			}
			result.Status = "partial_failure"
			result.Rollback = s.rollbackPatch(operations, undo, applied)
			result.Partial = result.Rollback != "complete"
			return result, err
		}
		result.Operations = append(result.Operations, item)
		if !mutated {
			continue
		}
		applied = append(applied, index)
		result.OperationsApplied++
		switch operation.Type {
		case PatchCreateFile, PatchCreateDirectory:
			result.Summary.Created++
		case PatchWriteFile:
			result.Summary.Updated++
		case PatchMovePath:
			result.Summary.Moved++
		case PatchDeleteFile:
			result.Summary.Deleted++
		}
	}
	result.Status = "applied"
	result.Rollback = "not_required"
	return result, nil
}

func validatePatchOperations(operations []PatchOperation) error {
	if len(operations) == 0 || len(operations) > MaxPatchOperations {
		return ErrPatchInvalid
	}
	contentBytes := 0
	for _, operation := range operations {
		switch operation.Type {
		case PatchCreateFile:
			if !validRelative(operation.Path) {
				return relativePathError(operation.Path)
			}
			if err := validatePatchText(operation.Content); err != nil {
				return err
			}
			contentBytes += len(operation.Content)
		case PatchWriteFile:
			if !validRelative(operation.Path) {
				return relativePathError(operation.Path)
			}
			if err := validatePatchText(operation.Content); err != nil {
				return err
			}
			if _, err := decodeSHA256(operation.ExpectedSHA256); err != nil {
				return err
			}
			contentBytes += len(operation.Content)
		case PatchMovePath:
			if !validRelative(operation.Source) || !validRelative(operation.Destination) {
				if reservedRelative(operation.Source) || reservedRelative(operation.Destination) {
					return ErrReservedPath
				}
				return ErrInvalidPath
			}
			if operation.Source == operation.Destination {
				return ErrPatchInvalid
			}
			if relativeDescendant(operation.Source, operation.Destination) {
				return ErrPatchConflict
			}
		case PatchDeleteFile:
			if !validRelative(operation.Path) {
				return relativePathError(operation.Path)
			}
			if _, err := decodeSHA256(operation.ExpectedSHA256); err != nil {
				return err
			}
		case PatchCreateDirectory:
			if !validRelative(operation.Path) {
				return relativePathError(operation.Path)
			}
		default:
			return ErrPatchInvalid
		}
		if contentBytes > MaxPatchContentBytes {
			return ErrTooLarge
		}
	}
	return nil
}

func validatePatchText(content string) error {
	if len(content) > MaxTextBytes {
		return ErrTooLarge
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return ErrNotText
	}
	return nil
}

func (s *Session) preflightPatch(operations []PatchOperation) ([]patchUndo, error) {
	undo := make([]patchUndo, len(operations))
	claimed := make([]string, 0, len(operations)*2)
	plannedDirectories := make(map[string]bool)
	for index, operation := range operations {
		endpoints := patchEndpoints(operation)
		for _, endpoint := range endpoints {
			for _, previous := range claimed {
				if !patchPathsOverlap(endpoint, previous) {
					continue
				}
				if endpoint == previous || !patchPlannedParentAllowed(previous, endpoint, operations, index) {
					return undo, ErrPatchConflict
				}
			}
		}
		for _, endpoint := range endpoints {
			claimed = append(claimed, endpoint)
		}

		switch operation.Type {
		case PatchCreateFile:
			if err := s.ensurePatchParent(operation.Path, plannedDirectories); err != nil {
				return undo, err
			}
			stat, err := s.statPatchPath(operation.Path, plannedDirectories)
			if err != nil {
				return undo, err
			}
			if stat.Found {
				if stat.Kind == FileKindSymlink {
					return undo, ErrUnsafePath
				}
				return undo, ErrPatchAlreadyExists
			}
		case PatchWriteFile, PatchDeleteFile:
			stat, err := s.statPatchPath(operation.Path, plannedDirectories)
			if err != nil {
				return undo, err
			}
			if !stat.Found {
				return undo, ErrPatchNotFound
			}
			if stat.Kind != FileKindRegular {
				if stat.Kind == FileKindSymlink {
					return undo, ErrUnsafePath
				}
				return undo, ErrNotFile
			}
			content, err := s.ReadText(operation.Path)
			if err != nil {
				return undo, err
			}
			actual := sha256Text(content)
			if actual != operation.ExpectedSHA256 {
				return undo, ErrPatchHashConflict
			}
			undo[index] = patchUndo{operation: operation, oldContent: content, oldSHA256: actual}
		case PatchMovePath:
			source, err := s.statPatchPath(operation.Source, plannedDirectories)
			if err != nil {
				return undo, err
			}
			if !source.Found {
				return undo, ErrPatchNotFound
			}
			if source.Kind != FileKindRegular && source.Kind != FileKindDirectory {
				if source.Kind == FileKindSymlink {
					return undo, ErrUnsafePath
				}
				return undo, ErrUnsupportedType
			}
			destination, err := s.statPatchPath(operation.Destination, plannedDirectories)
			if err != nil {
				return undo, err
			}
			if destination.Found {
				if destination.Kind == FileKindSymlink {
					return undo, ErrUnsafePath
				}
				return undo, ErrPatchAlreadyExists
			}
			if err := s.ensurePatchParent(operation.Destination, plannedDirectories); err != nil {
				return undo, err
			}
		case PatchCreateDirectory:
			if err := s.ensurePatchParent(operation.Path, plannedDirectories); err != nil {
				return undo, err
			}
			stat, err := s.statPatchPath(operation.Path, plannedDirectories)
			if err != nil {
				return undo, err
			}
			if stat.Found && stat.Kind != FileKindDirectory {
				if stat.Kind == FileKindSymlink {
					return undo, ErrUnsafePath
				}
				return undo, ErrPatchAlreadyExists
			}
			undo[index] = patchUndo{operation: operation, directoryNoOp: stat.Found}
			plannedDirectories[operation.Path] = stat.Found
		}
	}
	return undo, nil
}

func (s *Session) ensurePatchParent(relative string, planned map[string]bool) error {
	parent := relative
	if slash := strings.LastIndexByte(relative, '/'); slash >= 0 {
		parent = relative[:slash]
	} else {
		parent = "."
	}
	if parent != "." {
		if _, ok := planned[parent]; ok {
			return nil
		}
	}
	stat, err := s.StatPath(parent)
	if err != nil {
		return err
	}
	if !stat.Found {
		return ErrPatchNotFound
	}
	if stat.Kind != FileKindDirectory {
		return ErrUnsafePath
	}
	return nil
}

func (s *Session) statPatchPath(relative string, planned map[string]bool) (PathStat, error) {
	parent := relative
	for parent != "." {
		if existing, ok := planned[parent]; ok && !existing {
			return PathStat{Path: relative, Kind: FileKindAbsent, Found: false}, nil
		}
		if slash := strings.LastIndexByte(parent, '/'); slash >= 0 {
			parent = parent[:slash]
		} else {
			parent = "."
		}
	}
	return s.StatPath(relative)
}

func patchEndpoints(operation PatchOperation) []string {
	switch operation.Type {
	case PatchMovePath:
		return []string{operation.Source, operation.Destination}
	default:
		return []string{operation.Path}
	}
}

func patchPathsOverlap(left, right string) bool {
	return left == right || relativeDescendant(left, right) || relativeDescendant(right, left)
}

func patchPlannedParentAllowed(previous, current string, operations []PatchOperation, currentIndex int) bool {
	if !relativeDescendant(previous, current) {
		return false
	}
	for index := 0; index < currentIndex; index++ {
		if operations[index].Type == PatchCreateDirectory && operations[index].Path == previous {
			return true
		}
	}
	return false
}

func patchOperationResult(index int, operation PatchOperation) PatchOperationResult {
	result := PatchOperationResult{Index: index, Type: operation.Type, Status: "not_started"}
	if operation.Type == PatchMovePath {
		result.Source = operation.Source
		result.Destination = operation.Destination
	} else {
		result.Path = operation.Path
	}
	return result
}

func (s *Session) rollbackPatch(operations []PatchOperation, undo []patchUndo, applied []int) string {
	for position := len(applied) - 1; position >= 0; position-- {
		index := applied[position]
		operation := operations[index]
		var err error
		switch operation.Type {
		case PatchCreateFile:
			_, err = s.deleteFileExpected(operation.Path, sha256Text(operation.Content))
		case PatchCreateDirectory:
			if !undo[index].directoryNoOp {
				_, err = s.DeleteDirectory(operation.Path)
			}
		case PatchWriteFile:
			_, err = s.WriteTextFile(operation.Path, sha256Text(operation.Content), undo[index].oldContent)
		case PatchMovePath:
			_, err = s.Move(operation.Destination, operation.Source)
		case PatchDeleteFile:
			_, err = s.CreateTextFile(operation.Path, undo[index].oldContent)
		}
		if err != nil {
			return "partial"
		}
	}
	return "complete"
}

func (s *Session) deleteFileExpected(relative, expectedSHA256 string) (DeleteResult, error) {
	if _, err := decodeSHA256(expectedSHA256); err != nil {
		return DeleteResult{Path: relative}, err
	}
	content, err := s.ReadText(relative)
	if err != nil {
		return DeleteResult{Path: relative}, err
	}
	if sha256Text(content) != expectedSHA256 {
		return DeleteResult{Path: relative}, ErrPatchHashConflict
	}
	return s.DeleteFile(relative)
}

func patchStatus(err error) string {
	switch {
	case errors.Is(err, ErrReservedPath):
		return "reserved_path"
	case errors.Is(err, ErrInvalidPath):
		return "invalid_path"
	case errors.Is(err, ErrPatchInvalid), errors.Is(err, ErrInvalidHash), errors.Is(err, ErrNotText):
		return "invalid_patch"
	case errors.Is(err, ErrPatchHashConflict), errors.Is(err, ErrConflict):
		return "hash_conflict"
	case errors.Is(err, ErrPatchNotFound), errors.Is(err, os.ErrNotExist):
		return "not_found"
	case errors.Is(err, ErrPatchAlreadyExists), errors.Is(err, ErrPathExists):
		return "already_exists"
	case errors.Is(err, ErrTooLarge), errors.Is(err, ErrStructuralLimit):
		return "limit_exceeded"
	case errors.Is(err, ErrPatchConflict):
		return "conflict"
	case errors.Is(err, ErrUnsupportedType), errors.Is(err, ErrNotFile), errors.Is(err, ErrUnsafePath):
		return "type_mismatch"
	case errors.Is(err, ErrCrossDevice):
		return "unsupported_operation"
	case errors.Is(err, ErrOperationUnknown):
		return "unknown"
	case errors.Is(err, ErrNotAuthorized):
		return "unauthorized"
	case errors.Is(err, ErrClosed):
		return "unknown"
	default:
		return "internal"
	}
}

func patchErrorCode(err error) string {
	return patchStatus(err)
}

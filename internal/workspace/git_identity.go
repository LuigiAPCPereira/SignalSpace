package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	gitIdentityVersion  = 1
	gitIdentityFilename = "git_identity.json"
	maxGitIdentityName  = 128
	maxGitIdentityEmail = 254
)

var (
	ErrGitIdentityMissing = errors.New("Git identity is not configured")
	ErrGitIdentityInvalid = errors.New("Git identity is invalid")
)

// GitIdentity é a identidade local escolhida pelo proprietário. Ela nunca é
// aceita como argumento MCP e não é retornada por ferramentas públicas.
type GitIdentity struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type storedGitIdentity struct {
	Version     int    `json:"version"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func validateGitIdentity(identity GitIdentity) error {
	if !validGitIdentityText(identity.DisplayName, maxGitIdentityName, false) ||
		!validGitIdentityText(identity.Email, maxGitIdentityEmail, true) {
		return ErrGitIdentityInvalid
	}
	return nil
}

func validGitIdentityText(value string, maxBytes int, asciiOnly bool) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r == '<' || r == '>' || r == '\x00' || unicode.IsControl(r) {
			return false
		}
		if asciiOnly && r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func (m *ManagedWorktreeManager) gitIdentityPath() string {
	return filepath.Join(m.stateDir, gitIdentityFilename)
}

// GitIdentity retorna a identidade privada atualmente configurada.
func (m *ManagedWorktreeManager) GitIdentity() (GitIdentity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return GitIdentity{}, ErrManagedWorkspaceClosed
	}
	return m.gitIdentityLocked()
}

func (m *ManagedWorktreeManager) gitIdentityLocked() (GitIdentity, error) {
	data, err := readLimitedPrivate(m.gitIdentityPath(), 2<<10)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrManagedMetadata) {
			if _, statErr := os.Lstat(m.gitIdentityPath()); errors.Is(statErr, os.ErrNotExist) {
				return GitIdentity{}, ErrGitIdentityMissing
			}
		}
		return GitIdentity{}, ErrGitIdentityInvalid
	}
	var stored storedGitIdentity
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&stored) != nil || stored.Version != gitIdentityVersion {
		return GitIdentity{}, ErrGitIdentityInvalid
	}
	identity := GitIdentity{DisplayName: stored.DisplayName, Email: stored.Email}
	if err := validateGitIdentity(identity); err != nil {
		return GitIdentity{}, err
	}
	return identity, nil
}

// SetGitIdentity grava uma identidade privada, versionada e atomicamente.
func (m *ManagedWorktreeManager) SetGitIdentity(email, displayName string) error {
	identity := GitIdentity{DisplayName: displayName, Email: email}
	if err := validateGitIdentity(identity); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagedWorkspaceClosed
	}
	data, err := json.Marshal(storedGitIdentity{Version: gitIdentityVersion, DisplayName: identity.DisplayName, Email: identity.Email})
	if err != nil {
		return err
	}
	return writePrivateAtomic(m.stateDir, m.gitIdentityPath(), data)
}

// ClearGitIdentity desativa apenas futuras operações de commit. Ele não toca
// commits existentes, refs privadas ou escopos já concedidos.
func (m *ManagedWorktreeManager) ClearGitIdentity() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagedWorkspaceClosed
	}
	path := m.gitIdentityPath()
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return ErrGitIdentityInvalid
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func writePrivateAtomic(directory, path string, data []byte) error {
	tmp, err := os.CreateTemp(directory, ".signalspace-private-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return ErrGitIdentityInvalid
	}
	return os.Rename(tmpName, path)
}

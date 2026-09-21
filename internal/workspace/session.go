// Package workspace isola a leitura de arquivos dentro de uma raiz aprovada
// localmente. Nenhum endpoint remoto pode aprovar raízes por esta API.
package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"
)

const MaxTextBytes = 32 << 10

var (
	ErrInvalidRoot             = errors.New("invalid workspace root")
	ErrInvalidPath             = errors.New("invalid relative file path")
	ErrUnsafePath              = errors.New("symlink or non-directory path component")
	ErrNotFile                 = errors.New("path is not a regular file")
	ErrNotText                 = errors.New("file is not UTF-8 text")
	ErrTooLarge                = errors.New("file exceeds reading limit")
	ErrClosed                  = errors.New("workspace session closed")
	ErrInvalidProcessOperation = errors.New("invalid workspace process operation")
)

// Session mantém a identidade e o descritor de uma raiz aprovada em outra
// fronteira local. A criação não é uma autorização remota.
type Session struct {
	mu     sync.Mutex
	id     string
	rootFD int
	closed bool
}

// OpenApprovedRoot recebe somente uma raiz escolhida e aprovada pelo usuário
// local; o chamador não deve preenchê-la com dados vindos de uma ferramenta.
func OpenApprovedRoot(root string) (*Session, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" || strings.ContainsRune(root, '\x00') {
		return nil, ErrInvalidRoot
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if root == filepath.Clean(home) {
		return nil, ErrInvalidRoot
	}

	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(root, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			_ = syscall.Close(fd)
			return nil, ErrInvalidRoot
		}
		next, openErr := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		_ = syscall.Close(fd)
		if openErr != nil {
			return nil, normalizePathError(openErr)
		}
		fd = next
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return &Session{rootFD: fd, id: hex.EncodeToString(id[:])}, nil
}

func normalizePathError(err error) error {
	if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.ENOTDIR) {
		return ErrUnsafePath
	}
	return err
}

// ID é opaco e permanece constante somente durante a sessão.
func (s *Session) ID() string { return s.id }

// WithProcessDir executa uma operação local com o descritor da raiz retido sob
// o mutex da sessão. Isso impede que Close/Grant/Revogação feche e reutilize o
// FD entre a resolução de /proc/self/fd e o início do processo. O diretório
// não é um sandbox: o processo mantém os privilégios do usuário.
func (s *Session) WithProcessDir(operation func(string) error) error {
	if operation == nil {
		return ErrInvalidProcessOperation
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	return operation(fmt.Sprintf("/proc/self/fd/%d", s.rootFD))
}

func validRelative(relative string) bool {
	if relative == "" || relative == "." || len(relative) > 4096 || filepath.IsAbs(relative) ||
		filepath.Clean(relative) != relative || strings.ContainsAny(relative, "\\\x00") || !utf8.ValidString(relative) {
		return false
	}
	for _, part := range strings.Split(relative, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// ReadText recusa links simbólicos em todos os componentes. Os descritores
// relativos à raiz impedem troca por symlink entre validação e abertura.
func (s *Session) ReadText(relative string) (string, error) {
	if !validRelative(relative) {
		return "", ErrInvalidPath
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrClosed
	}

	parts := strings.Split(relative, "/")
	fd := s.rootFD
	for _, part := range parts[:len(parts)-1] {
		next, err := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if fd != s.rootFD {
			_ = syscall.Close(fd)
		}
		if err != nil {
			return "", normalizePathError(err)
		}
		fd = next
	}
	opened, err := syscall.Openat(fd, parts[len(parts)-1], syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if fd != s.rootFD {
		_ = syscall.Close(fd)
	}
	if err != nil {
		return "", normalizePathError(err)
	}
	file := os.NewFile(uintptr(opened), "workspace-file")
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrNotFile
	}
	if info.Size() > MaxTextBytes {
		return "", ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxTextBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > MaxTextBytes {
		return "", ErrTooLarge
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
		return "", ErrNotText
	}
	return string(data), nil
}

// Close encerra a autorização de leitura desta sessão e seu descritor.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return syscall.Close(s.rootFD)
}

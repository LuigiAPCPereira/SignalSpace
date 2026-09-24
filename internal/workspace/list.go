package workspace

import (
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"
)

const MaxDirectoryEntries = 128

var ErrTooManyEntries = errors.New("directory exceeds listing limit")

// ListDirectory devolve somente nomes: não lê conteúdos, não classifica tipos
// potencialmente obsoletos e não segue links simbólicos. "." indica a raiz
// aprovada; os demais caminhos devem ser relativos e canônicos.
func (s *Session) ListDirectory(relative string) ([]string, error) {
	if relative != "." && !validRelative(relative) {
		return nil, relativePathError(relative)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}

	fd := s.rootFD
	if relative != "." {
		for _, part := range strings.Split(relative, "/") {
			next, err := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
			if fd != s.rootFD {
				_ = syscall.Close(fd)
			}
			if err != nil {
				return nil, normalizePathError(err)
			}
			fd = next
		}
	} else {
		// dup compartilharia o offset de leitura da raiz com a sessão.
		opened, err := syscall.Openat(fd, ".", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if err != nil {
			return nil, normalizePathError(err)
		}
		fd = opened
	}

	directory := os.NewFile(uintptr(fd), "workspace-directory")
	defer directory.Close()
	// Readdirnames evita stat implícito por um pathname externo à raiz.
	names, err := directory.Readdirnames(MaxDirectoryEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > MaxDirectoryEntries {
		return nil, ErrTooManyEntries
	}
	for _, name := range names {
		if !utf8.ValidString(name) || name == "." || name == ".." || strings.ContainsRune(name, '\x00') {
			return nil, ErrInvalidPath
		}
	}
	filtered := names[:0]
	for _, name := range names {
		if name != ".git" {
			filtered = append(filtered, name)
		}
	}
	sort.Strings(filtered)
	return filtered, nil
}

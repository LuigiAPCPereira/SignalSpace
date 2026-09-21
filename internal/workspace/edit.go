package workspace

import (
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
)

var ErrConflict = errors.New("workspace file changed since the expected version")

// ReplaceText substitui um arquivo de texto somente quando o conteúdo atual
// coincide exatamente com expected. A operação é serializada com leitura,
// listagem e revogação dentro desta sessão; nenhum endpoint remoto cria uma
// concessão ou amplia seu escopo.
//
// A substituição preserva o inode e as permissões existentes. O chamador deve
// fornecer expected novamente em cada alteração, evitando overwrite silencioso
// quando outra operação desta instância já modificou o arquivo.
func (s *Session) ReplaceText(relative, expected, replacement string) error {
	if !validRelative(relative) {
		return ErrInvalidPath
	}
	if !validTextContent(expected) || !validTextContent(replacement) {
		return ErrNotText
	}
	if len(expected) > MaxTextBytes || len(replacement) > MaxTextBytes {
		return ErrTooLarge
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}

	parts := strings.Split(relative, "/")
	fd := s.rootFD
	for _, part := range parts[:len(parts)-1] {
		next, err := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if fd != s.rootFD {
			_ = syscall.Close(fd)
		}
		if err != nil {
			return normalizePathError(err)
		}
		fd = next
	}
	opened, err := syscall.Openat(fd, parts[len(parts)-1], syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if fd != s.rootFD {
		_ = syscall.Close(fd)
	}
	if err != nil {
		return normalizePathError(err)
	}
	file := os.NewFile(uintptr(opened), "workspace-file")
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrNotFile
	}
	if info.Size() > MaxTextBytes {
		return ErrTooLarge
	}
	current, err := io.ReadAll(io.LimitReader(file, MaxTextBytes+1))
	if err != nil {
		return err
	}
	if len(current) > MaxTextBytes {
		return ErrTooLarge
	}
	if !validTextContent(string(current)) {
		return ErrNotText
	}
	if string(current) != expected {
		return ErrConflict
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if err := writeAll(file, []byte(replacement)); err != nil {
		return err
	}
	return nil
}

func validTextContent(content string) bool {
	return utf8.ValidString(content) && !strings.ContainsRune(content, '\x00')
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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
// A substituição é preparada em arquivo temporário no mesmo diretório e
// publicada atomicamente, preservando as permissões existentes. O chamador
// deve fornecer expected novamente em cada alteração, evitando overwrite
// silencioso quando outra operação desta instância já modificou o arquivo.
func (s *Session) ReplaceText(relative, expected, replacement string) error {
	if !validRelative(relative) {
		return relativePathError(relative)
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
	parentFD := s.rootFD
	for _, part := range parts[:len(parts)-1] {
		next, err := syscall.Openat(parentFD, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if parentFD != s.rootFD {
			_ = syscall.Close(parentFD)
		}
		if err != nil {
			return normalizePathError(err)
		}
		parentFD = next
	}
	if parentFD != s.rootFD {
		defer syscall.Close(parentFD)
	}
	name := parts[len(parts)-1]
	opened, err := syscall.Openat(parentFD, name, syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
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

	tempName, err := newEditTempName()
	if err != nil {
		return err
	}
	tempFD, err := syscall.Openat(parentFD, tempName, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, uint32(info.Mode().Perm()))
	if err != nil {
		return err
	}
	temp := os.NewFile(uintptr(tempFD), "workspace-edit-temp")
	removeTemp := true
	defer func() {
		if temp != nil {
			_ = temp.Close()
		}
		if removeTemp {
			_ = syscall.Unlinkat(parentFD, tempName)
		}
	}()
	if err := syscall.Fchmod(int(temp.Fd()), uint32(info.Mode().Perm())); err != nil {
		return err
	}
	if err := writeAll(temp, []byte(replacement)); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	temp = nil

	// A troca externa do caminho entre a leitura e a publicação vira conflito
	// quando observável; o rename nunca segue um symlink como diretório-alvo.
	currentFD, err := syscall.Openat(parentFD, name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return normalizePathError(err)
	}
	currentFile := os.NewFile(uintptr(currentFD), "workspace-current")
	currentInfo, statErr := currentFile.Stat()
	closeErr := currentFile.Close()
	if statErr != nil {
		return statErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !os.SameFile(info, currentInfo) {
		return ErrConflict
	}
	if err := syscall.Renameat(parentFD, tempName, parentFD, name); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func newEditTempName() (string, error) {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf(".signalspace-edit-%s", hex.EncodeToString(nonce[:])), nil
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

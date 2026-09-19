package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
)

const (
	stateVersion  = 1
	maxStateBytes = 2 << 20
	stateFileName = "identity.json"
)

type storedIdentity struct {
	Version     int               `json:"version"`
	Issuer      string            `json:"issuer"`
	ResourceURL string            `json:"resource_url"`
	KeyID       string            `json:"key_id"`
	PrivateKey  string            `json:"private_key"`
	Clients     map[string]client `json:"clients"`
}

type identityStore struct {
	dir  string
	path string
	lock *os.File
}

// openIdentity vincula a chave e os clientes à URL do recurso configurado.
// Não recupera códigos ou aprovações: esses estados expiram com o processo.
func openIdentity(dir string, config Config) (*identityStore, *rsa.PrivateKey, string, map[string]client, error) {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || dir == string(filepath.Separator) {
		return nil, nil, "", nil, errors.New("OAuth state directory must be a canonical absolute path")
	}
	if err := ensurePrivateDirectory(dir); err != nil {
		return nil, nil, "", nil, err
	}
	store := &identityStore{dir: dir, path: filepath.Join(dir, stateFileName)}
	if err := store.lockDirectory(); err != nil {
		return nil, nil, "", nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = store.Close()
		}
	}()
	data, err := store.read()
	if errors.Is(err, os.ErrNotExist) {
		key, keyErr := rsa.GenerateKey(rand.Reader, 2048)
		if keyErr != nil {
			return nil, nil, "", nil, keyErr
		}
		kid, keyErr := randomID(16)
		if keyErr != nil {
			return nil, nil, "", nil, keyErr
		}
		clients := make(map[string]client)
		if err := store.saveInitial(config, key, kid, clients); err != nil {
			return nil, nil, "", nil, err
		}
		ok = true
		return store, key, kid, clients, nil
	}
	if err != nil {
		return nil, nil, "", nil, err
	}
	var state storedIdentity
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, nil, "", nil, errors.New("OAuth identity file is malformed")
	}
	if state.Version != stateVersion || state.Issuer != config.Issuer || state.ResourceURL != config.ResourceURL {
		return nil, nil, "", nil, errors.New("OAuth identity version or public URL changed; explicit migration is required")
	}
	if !requestID.MatchString(state.KeyID) || len(state.Clients) > maxClients || state.Clients == nil {
		return nil, nil, "", nil, errors.New("OAuth identity contains invalid key or client metadata")
	}
	for id, c := range state.Clients {
		if !validStoredClient(id, c) {
			return nil, nil, "", nil, errors.New("OAuth identity contains invalid client metadata")
		}
	}
	der, err := base64.StdEncoding.DecodeString(state.PrivateKey)
	if err != nil || len(der) > 8192 {
		return nil, nil, "", nil, errors.New("OAuth private key is malformed")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, nil, "", nil, errors.New("OAuth private key is malformed")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() != 2048 || key.Validate() != nil {
		return nil, nil, "", nil, errors.New("OAuth private key is invalid")
	}
	ok = true
	return store, key, state.KeyID, state.Clients, nil
}

func validStoredClient(id string, c client) bool {
	if len(id) != 32 || c.Name == "" || len(c.Name) > 100 || strings.TrimSpace(c.Name) != c.Name || strings.IndexFunc(c.Name, unicode.IsControl) >= 0 || len(c.Redirects) < 1 || len(c.Redirects) > 5 {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	for _, redirect := range c.Redirects {
		if len(redirect) > 2048 || !allowedRedirect(redirect) {
			return false
		}
	}
	return true
}

// Uma trava de processo impede duas instâncias de sobrescrever clientes ou chaves.
func (s *identityStore) lockDirectory() error {
	path := filepath.Join(s.dir, ".lock")
	// O_NOFOLLOW impede que o arquivo de trava seja substituído por um symlink.
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return fmt.Errorf("open OAuth state lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ownedByCurrentUser(info) {
		file.Close()
		return errors.New("OAuth state lock must be a private regular file")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return errors.New("OAuth state directory is already in use")
	}
	s.lock = file
	return nil
}

func (s *identityStore) Close() error {
	if s.lock == nil {
		return nil
	}
	err := s.lock.Close()
	s.lock = nil
	return err
}

// ensurePrivateDirectory rejeita symlinks em todos os componentes do caminho.
// A pasta final pertence exclusivamente ao usuário que executa o serviço.
func ensurePrivateDirectory(dir string) error {
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(dir, current), current) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0700); err != nil {
				return fmt.Errorf("create OAuth state directory: %w", err)
			}
			info, err = os.Lstat(current)
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("OAuth state path contains a symlink or is not a directory")
		}
	}
	info, err := os.Lstat(dir)
	if err != nil || info.Mode().Perm() != 0700 || !ownedByCurrentUser(info) {
		return errors.New("OAuth state directory must be owned by this user with permissions 0700")
	}
	return nil
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}

func (s *identityStore) read() ([]byte, error) {
	info, err := os.Lstat(s.path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ownedByCurrentUser(info) || info.Size() > maxStateBytes {
		return nil, errors.New("OAuth identity must be a user-owned regular file with permissions 0600")
	}
	fd, err := syscall.Open(s.path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), s.path)
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0600 || !ownedByCurrentUser(opened) || !sameIdentityFile(info, opened) {
		return nil, errors.New("OAuth identity changed during read")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStateBytes+1))
	if err != nil || len(data) > maxStateBytes {
		return nil, errors.New("OAuth identity read failed or exceeds size limit")
	}
	return data, nil
}

func sameIdentityFile(before, after os.FileInfo) bool {
	a, aOK := before.Sys().(*syscall.Stat_t)
	b, bOK := after.Sys().(*syscall.Stat_t)
	return aOK && bOK && a.Dev == b.Dev && a.Ino == b.Ino
}

func (s *identityStore) encode(config Config, key *rsa.PrivateKey, kid string, clients map[string]client) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return json.Marshal(storedIdentity{Version: stateVersion, Issuer: config.Issuer, ResourceURL: config.ResourceURL, KeyID: kid, PrivateKey: base64.StdEncoding.EncodeToString(der), Clients: clients})
}

func (s *identityStore) saveInitial(config Config, key *rsa.PrivateKey, kid string, clients map[string]client) error {
	data, err := s.encode(config, key, kid, clients)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create OAuth identity without replacing existing file: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return syncDirectory(s.dir)
}

// save substitui o arquivo por rename atômico, antes de autorizar o cliente em memória.
func (s *identityStore) save(config Config, key *rsa.PrivateKey, kid string, clients map[string]client) error {
	if _, err := s.read(); err != nil {
		return fmt.Errorf("OAuth identity is not safe to update: %w", err)
	}
	data, err := s.encode(config, key, kid, clients)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".identity-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), s.path); err != nil {
		return err
	}
	return syncDirectory(s.dir)
}

func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

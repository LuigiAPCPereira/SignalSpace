package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestConcurrentWorkspaceReadListRevocation exercita a disputa entre chamadas
// de leitura/listagem e revogação sem pressupor qual operação obterá o mutex.
// Depois que Revoke retorna, nenhuma chamada nova com o ID antigo pode ter êxito.
func TestConcurrentWorkspaceReadListRevocation(t *testing.T) {
	grants, err := NewGrants("owner")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "approved.txt"), []byte("approved content"), 0600); err != nil {
		t.Fatal(err)
	}
	sessionID, err := grants.Grant(root, testClientA)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 32
	start := make(chan struct{})
	failures := make(chan error, workers)
	var pending sync.WaitGroup
	for i := 0; i < workers; i++ {
		pending.Add(1)
		go func(index int) {
			defer pending.Done()
			<-start
			if index%2 == 0 {
				content, readErr := grants.ReadText("owner", testClientA, sessionID, "approved.txt")
				if readErr == nil && content == "approved content" {
					return
				}
				if errors.Is(readErr, ErrNotAuthorized) && content == "" {
					return
				}
				failures <- fmt.Errorf("concurrent read: content=%q err=%v", content, readErr)
				return
			}
			names, listErr := grants.ListDirectory("owner", testClientA, sessionID, ".")
			if listErr == nil && len(names) == 1 && names[0] == "approved.txt" {
				return
			}
			if errors.Is(listErr, ErrNotAuthorized) && names == nil {
				return
			}
			failures <- fmt.Errorf("concurrent listing: names=%v err=%v", names, listErr)
		}(i)
	}
	close(start)
	if err := grants.Revoke(sessionID); err != nil {
		t.Fatal(err)
	}
	pending.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}

	// A segunda fase começa estritamente depois do retorno de Revoke.
	// Ela não pode observar dados, mesmo com várias chamadas simultâneas.
	postRevoke := make(chan struct{})
	// Cada goroutine pode registrar duas falhas sem bloquear o próprio teste.
	denials := make(chan error, workers*2)
	var denied sync.WaitGroup
	for i := 0; i < workers; i++ {
		denied.Add(1)
		go func() {
			defer denied.Done()
			<-postRevoke
			content, readErr := grants.ReadText("owner", testClientA, sessionID, "approved.txt")
			if !errors.Is(readErr, ErrNotAuthorized) || content != "" {
				denials <- fmt.Errorf("read after revoke: content=%q err=%v", content, readErr)
			}
			names, listErr := grants.ListDirectory("owner", testClientA, sessionID, ".")
			if !errors.Is(listErr, ErrNotAuthorized) || names != nil {
				denials <- fmt.Errorf("listing after revoke: names=%v err=%v", names, listErr)
			}
		}()
	}
	close(postRevoke)
	denied.Wait()
	close(denials)
	for failure := range denials {
		t.Error(failure)
	}

	// Uma nova concessão não ressuscita a sessão anterior nem troca seu cliente.
	replacement, err := grants.Grant(root, testClientB)
	if err != nil || replacement == sessionID {
		t.Fatalf("replacement: id=%q err=%v", replacement, err)
	}
	if names, err := grants.ListDirectory("owner", testClientA, sessionID, "."); !errors.Is(err, ErrNotAuthorized) || names != nil {
		t.Fatalf("old session became valid after replacement: %v %v", names, err)
	}
	if names, err := grants.ListDirectory("owner", testClientA, replacement, "."); !errors.Is(err, ErrNotAuthorized) || names != nil {
		t.Fatalf("old client reused new session: %v %v", names, err)
	}
	if names, err := grants.ListDirectory("owner", testClientB, replacement, "."); err != nil || len(names) != 1 || names[0] != "approved.txt" {
		t.Fatalf("replacement session denied: %v %v", names, err)
	}
}

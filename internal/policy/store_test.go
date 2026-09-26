package policy

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

func TestStoreIsPrivateAtomicAndReloadable(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "state", "policies.json")
	store, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Upsert(StoredPolicy{OwnerID: "owner", ClientID: "client", WorkspaceID: "managed", Capability: capability.WorkspaceWrite, Effect: EffectAllowWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
		t.Fatalf("generated policy metadata = %+v", item)
	}
	info, err := os.Stat(storePath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("policy file permissions = %v err=%v", info.Mode().Perm(), err)
	}
	reloaded, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	items, err := reloaded.List()
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("reloaded policies = %+v err=%v", items, err)
	}
	if err := reloaded.Delete(item.ID); err != nil {
		t.Fatal(err)
	}
	items, err = store.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("deleted policies = %+v err=%v", items, err)
	}
}

func TestStoreSerializesConcurrentWriters(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "policies.json")
	first, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			_, _ = first.Upsert(StoredPolicy{OwnerID: "owner", ClientID: "client", WorkspaceID: "managed-" + string(rune('a'+index)), Capability: capability.WorkspaceWrite, Effect: EffectAllowWorkspace})
		}(index)
		group.Add(1)
		go func(index int) {
			defer group.Done()
			_, _ = second.Upsert(StoredPolicy{OwnerID: "owner", ClientID: "client", WorkspaceID: "other-" + string(rune('a'+index)), Capability: capability.GitReview, Effect: EffectAllowWorkspace})
		}(index)
	}
	group.Wait()
	items, err := first.List()
	if err != nil || len(items) != 16 {
		t.Fatalf("concurrent policies = %d err=%v", len(items), err)
	}
}

func TestStoreFailsClosedOnMalformedOrUnknownData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"policies":[{"id":"bad","owner_id":"owner","client_id":"client","workspace_id":"managed","capability":"workspace.write","effect":"ALLOW_WORKSPACE","created_at":"`+time.Now().UTC().Format(time.RFC3339Nano)+`","updated_at":"`+time.Now().UTC().Format(time.RFC3339Nano)+`","unexpected":"value"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); !errors.Is(err, ErrInvalidStore) {
		t.Fatalf("unknown field error = %v", err)
	}
}

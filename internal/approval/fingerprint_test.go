package approval

import (
	"errors"
	"strings"
	"testing"
)

func TestCanonicalFingerprintIsStableAndOperationBound(t *testing.T) {
	first, err := CanonicalFingerprint("write_text_file", "session", map[string]any{
		"path": "src/main.go", "content_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalFingerprint("write_text_file", "session", map[string]any{
		"content_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "path": "src/main.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 || strings.ToLower(first) != first {
		t.Fatalf("non-canonical fingerprint: first=%q second=%q", first, second)
	}
	changed, err := CanonicalFingerprint("write_text_file", "session", map[string]any{"path": "src/other.go"})
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("changed operation reused fingerprint")
	}
	if _, err := CanonicalFingerprint("", "session", nil); !errors.Is(err, ErrInvalidFingerprintInput) {
		t.Fatalf("invalid tool error = %v", err)
	}
}

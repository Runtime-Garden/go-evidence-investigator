package casebook

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateReviewConflictAndIntegrity(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.Create(map[string]any{"claim": "The leaf absorbs sunlight"}, "REQUIRES_REVIEW")
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 1 || c.Status != "pending_review" {
		t.Fatal(c)
	}
	got, err := store.Get(c.ID)
	if err != nil || got.ID != c.ID {
		t.Fatalf("get %v %v", got, err)
	}
	c, err = store.Review(c.ID, 1, "approved", "researcher", "Checked citations against source documents")
	if err != nil || c.Status != "approved" || c.Version != 2 || len(c.Events) != 2 {
		t.Fatalf("review %v %v", c, err)
	}
	if _, err = store.Review(c.ID, 1, "rejected", "researcher", "stale"); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected stale conflict: %v", err)
	}
	if _, err = store.Review(c.ID, 2, "rejected", "researcher", "cannot undo"); err == nil {
		t.Fatal("allowed post-approval review")
	}
	data, err := os.ReadFile(filepath.Join(store.Dir, c.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	corrupt := strings.Replace(string(data), "Checked citations against source documents", "unverified edit", 1)
	if err := os.WriteFile(filepath.Join(store.Dir, c.ID+".json"), []byte(corrupt), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(c.ID); err == nil {
		t.Fatal("tampered review chain accepted")
	}
}
func TestRejectTraversal(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("../../etc/passwd"); err == nil {
		t.Fatal("accepted traversal")
	}
}

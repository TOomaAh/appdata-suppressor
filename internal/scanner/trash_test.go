package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrashLifecycle(t *testing.T) {
	ctx := context.Background()
	s := setup(t, nil)

	if res := s.Delete(ctx, []string{"old", TrashDirName}); res[0].Error != "" || res[1].Error == "" {
		t.Fatalf("delete results = %+v; want old trashed and the trash dir refused", res)
	}

	// The trash dir is not listed as an appdata folder.
	report, err := s.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Folders {
		if f.Name == "old" || f.Name == TrashDirName {
			t.Errorf("scan lists %q", f.Name)
		}
	}

	items, err := s.Trash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "old" || items[0].Size != 100 || items[0].DeletedAt.IsZero() {
		t.Fatalf("trash = %+v; want old (100 bytes) with a deletion time", items)
	}

	// Restoring refuses to overwrite a folder recreated meanwhile.
	recreated := filepath.Join(s.Root, "old")
	if err := os.Mkdir(recreated, 0o755); err != nil {
		t.Fatal(err)
	}
	if res := s.Restore([]string{items[0].ID}); res[0].Error == "" {
		t.Error("restore over an existing folder should fail")
	}
	if err := os.Remove(recreated); err != nil {
		t.Fatal(err)
	}
	if res := s.Restore([]string{items[0].ID}); res[0].Error != "" {
		t.Fatalf("restore: %s", res[0].Error)
	}
	if _, err := os.Stat(filepath.Join(recreated, "f")); err != nil {
		t.Error("restored folder lost its content")
	}

	// Purge deletes permanently and rejects ids that are paths.
	s.Delete(ctx, []string{"old"})
	items, _ = s.Trash(ctx)
	if res := s.Purge([]string{items[0].ID, "../plex"}); res[0].Error != "" || res[1].Error == "" {
		t.Fatalf("purge results = %+v", res)
	}
	if items, _ = s.Trash(ctx); len(items) != 0 {
		t.Errorf("trash not empty after purge: %+v", items)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "plex")); err != nil {
		t.Error("purge escaped the trash dir")
	}
}

func TestPurgeExpired(t *testing.T) {
	s := setup(t, nil)
	trash := s.trashDir()
	old := time.Now().Add(-10*24*time.Hour).Format(trashTimeLayout) + "_old"
	recent := time.Now().Add(-time.Hour).Format(trashTimeLayout) + "_recent"
	for _, id := range []string{old, recent, "added-by-hand"} {
		if err := os.MkdirAll(filepath.Join(trash, id), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	purged, err := s.PurgeExpired(context.Background(), 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(purged) != 1 || purged[0] != old {
		t.Fatalf("purged = %v; want only %s", purged, old)
	}
	items, _ := s.Trash(context.Background())
	if len(items) != 2 || items[0].Name != "recent" || items[1].Name != "added-by-hand" {
		t.Errorf("remaining trash = %+v; want recent then added-by-hand", items)
	}
}

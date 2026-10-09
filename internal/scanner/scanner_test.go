package scanner

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"appdata-suppressor/internal/docker"
)

type fakeDocker []docker.Container

func (f fakeDocker) Containers(context.Context) ([]docker.Container, error) { return f, nil }

func TestTopLevelFolder(t *testing.T) {
	roots := []string{"/mnt/cache/appdata", "/mnt/user/appdata"}
	tests := []struct {
		src  string
		want string
		ok   bool
	}{
		{"/mnt/cache/appdata/plex", "plex", true},
		{"/mnt/cache/appdata/plex/config/", "plex", true},
		{"/mnt/user/appdata/sonarr/data", "sonarr", true},
		{"/mnt/cache/appdata", "", false},
		{"/mnt/cache/appdata-old/plex", "", false},
		{"/mnt/user/media", "", false},
	}
	for _, tt := range tests {
		got, ok := topLevelFolder(tt.src, roots)
		if got != tt.want || ok != tt.ok {
			t.Errorf("topLevelFolder(%q) = %q, %v; want %q, %v", tt.src, got, ok, tt.want, tt.ok)
		}
	}
}

func setup(t *testing.T, mounts map[string][]string) *Scanner {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"plex", "sonarr", "old"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f"), make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var containers fakeDocker
	for name, paths := range mounts {
		c := docker.Container{Name: name}
		for _, p := range paths {
			c.Mounts = append(c.Mounts, filepath.Join(root, p))
		}
		containers = append(containers, c)
	}
	return &Scanner{Root: root, Docker: containers}
}

func TestScan(t *testing.T) {
	s := setup(t, map[string][]string{
		"plex":   {"plex/config", "plex/transcode"},
		"sonarr": {"sonarr"},
		"backup": {"."}, // mounting the whole root marks nothing as used
	})
	report, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.OrphanCount != 1 || report.UsedCount != 2 {
		t.Fatalf("orphans=%d used=%d, want 1 and 2", report.OrphanCount, report.UsedCount)
	}
	if report.TotalSize != 300 || report.OrphanSize != 100 {
		t.Fatalf("total=%d orphan=%d, want 300 and 100", report.TotalSize, report.OrphanSize)
	}
	for _, f := range report.Folders {
		switch f.Name {
		case "plex":
			if !slices.Equal(f.Containers, []string{"plex"}) {
				t.Errorf("plex containers = %v", f.Containers)
			}
		case "old":
			if !f.Orphan {
				t.Error("old should be orphan")
			}
		}
	}
}

func TestDelete(t *testing.T) {
	s := setup(t, map[string][]string{"plex": {"plex"}})
	results := s.Delete(context.Background(), []string{"old", "plex", "..", "missing"})

	if results[0].Error != "" {
		t.Errorf("deleting orphan failed: %s", results[0].Error)
	}
	for _, r := range results[1:] {
		if r.Error == "" {
			t.Errorf("deleting %q should fail", r.Name)
		}
	}
	if _, err := os.Stat(filepath.Join(s.Root, "old")); !os.IsNotExist(err) {
		t.Error("old still in appdata")
	}
	if _, err := os.Stat(filepath.Join(s.Root, "plex")); err != nil {
		t.Error("plex was deleted")
	}
}

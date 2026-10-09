package scanner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// TrashDirName is the folder of Root where deleted folders are moved. Keeping it
// on the same filesystem makes moving a folder an instant rename.
const TrashDirName = ".appdata-trash"

// Trash entries are named "<deletion time>_<original name>".
const trashTimeLayout = "20060102-150405.000"

// TrashItem is a folder waiting in the trash.
type TrashItem struct {
	ID        string    `json:"id"` // entry name inside the trash dir
	Name      string    `json:"name"`
	DeletedAt time.Time `json:"deleted_at"`
	Size      int64     `json:"size"`
}

func (s *Scanner) trashDir() string { return filepath.Join(s.Root, TrashDirName) }

func (s *Scanner) moveToTrash(name string) error {
	if err := os.MkdirAll(s.trashDir(), 0o755); err != nil {
		return fmt.Errorf("create trash: %w", err)
	}
	id := time.Now().Format(trashTimeLayout) + "_" + name
	return os.Rename(filepath.Join(s.Root, name), filepath.Join(s.trashDir(), id))
}

// Trash lists the folders in the trash, most recently deleted first.
func (s *Scanner) Trash(ctx context.Context) ([]TrashItem, error) {
	entries, err := os.ReadDir(s.trashDir())
	if errors.Is(err, fs.ErrNotExist) {
		return []TrashItem{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read trash: %w", err)
	}

	items := []TrashItem{}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		item := parseTrashID(e.Name())
		items = append(items, item)
		paths = append(paths, filepath.Join(s.trashDir(), item.ID))
	}

	sizes := dirSizes(ctx, paths, nil)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Size = sizes[i]
	}
	slices.SortFunc(items, func(a, b TrashItem) int {
		return cmp.Or(b.DeletedAt.Compare(a.DeletedAt), cmp.Compare(a.ID, b.ID))
	})
	return items, nil
}

// Restore moves trashed folders back to their original name in Root.
func (s *Scanner) Restore(ids []string) []Result {
	return s.eachTrashItem(ids, func(item TrashItem, path string) error {
		dest := filepath.Join(s.Root, item.Name)
		if _, err := os.Lstat(dest); err == nil {
			return fmt.Errorf("%q already exists in appdata", item.Name)
		}
		return os.Rename(path, dest)
	})
}

// Purge permanently deletes trashed folders.
func (s *Scanner) Purge(ids []string) []Result {
	return s.eachTrashItem(ids, func(_ TrashItem, path string) error {
		return os.RemoveAll(path)
	})
}

// PurgeExpired permanently deletes folders trashed more than maxAge ago and
// returns their IDs. Entries whose deletion time cannot be parsed are kept.
func (s *Scanner) PurgeExpired(ctx context.Context, maxAge time.Duration) ([]string, error) {
	entries, err := os.ReadDir(s.trashDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read trash: %w", err)
	}
	var purged []string
	var errs []error
	for _, e := range entries {
		item := parseTrashID(e.Name())
		if !e.IsDir() || item.DeletedAt.IsZero() || time.Since(item.DeletedAt) < maxAge {
			continue
		}
		if err := ctx.Err(); err != nil {
			return purged, err
		}
		if err := os.RemoveAll(filepath.Join(s.trashDir(), item.ID)); err != nil {
			errs = append(errs, err)
			continue
		}
		purged = append(purged, item.ID)
	}
	return purged, errors.Join(errs...)
}

func (s *Scanner) eachTrashItem(ids []string, fn func(TrashItem, string) error) []Result {
	results := make([]Result, len(ids))
	for i, id := range ids {
		results[i].Name = id
		err := func() error {
			if !validName(id) {
				return fmt.Errorf("invalid trash entry %q", id)
			}
			path := filepath.Join(s.trashDir(), id)
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return fmt.Errorf("%q is not a directory", id)
			}
			return fn(parseTrashID(id), path)
		}()
		if err != nil {
			results[i].Error = err.Error()
		}
	}
	return results
}

// parseTrashID splits "<time>_<name>". An entry not following that pattern
// (e.g. added by hand) keeps its full name and a zero DeletedAt.
func parseTrashID(id string) TrashItem {
	item := TrashItem{ID: id, Name: id}
	ts, name, ok := strings.Cut(id, "_")
	if !ok || name == "" {
		return item
	}
	t, err := time.ParseInLocation(trashTimeLayout, ts, time.Local)
	if err != nil {
		return item
	}
	item.Name = name
	item.DeletedAt = t
	return item
}

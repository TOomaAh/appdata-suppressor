// Package scanner finds appdata folders that no Docker container bind-mounts anymore.
package scanner

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"appdata-suppressor/internal/docker"
)

// ContainerLister is implemented by docker.CLI; tests substitute a fake.
type ContainerLister interface {
	Containers(ctx context.Context) ([]docker.Container, error)
}

// Folder is a top-level directory of the appdata root.
type Folder struct {
	Name       string   `json:"name"`
	Size       int64    `json:"size"`
	Containers []string `json:"containers"`
	Orphan     bool     `json:"orphan"`
	// SizePending is set while the size is being computed; Size then holds the
	// value from the previous scan, if any.
	SizePending bool `json:"size_pending,omitempty"`
}

// Report is the result of a scan.
type Report struct {
	Root        string    `json:"root"`
	Folders     []Folder  `json:"folders"`
	TotalSize   int64     `json:"total_size"`
	OrphanSize  int64     `json:"orphan_size"`
	OrphanCount int       `json:"orphan_count"`
	UsedCount   int       `json:"used_count"`
	ScannedAt   time.Time `json:"scanned_at"`
}

// recount recomputes the totals from the folders.
func (r *Report) recount() {
	r.TotalSize, r.OrphanSize, r.OrphanCount, r.UsedCount = 0, 0, 0, 0
	for _, f := range r.Folders {
		r.TotalSize += f.Size
		if f.Orphan {
			r.OrphanCount++
			r.OrphanSize += f.Size
		} else {
			r.UsedCount++
		}
	}
}

// Result reports the outcome of an operation on one folder.
type Result struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

type Scanner struct {
	Root string
	// Aliases are other paths that point to Root, e.g. /mnt/user/appdata on
	// Unraid when Root is /mnt/cache/appdata. Mounts under them count as usage.
	Aliases []string
	Docker  ContainerLister
}

// Scan lists the folders of Root with their size and the containers using them.
func (s *Scanner) Scan(ctx context.Context) (*Report, error) {
	report, err := s.listFolders(ctx)
	if err != nil {
		return nil, err
	}
	sizes := dirSizes(ctx, s.folderPaths(report), nil)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i := range report.Folders {
		report.Folders[i].Size = sizes[i]
	}
	report.recount()
	return report, nil
}

// listFolders returns the folders of Root and the containers using them,
// without sizes, which are the slow part of a scan.
func (s *Scanner) listFolders(ctx context.Context) (*Report, error) {
	usage, err := s.folderUsage(ctx)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, fmt.Errorf("read appdata dir: %w", err)
	}

	report := &Report{Root: s.Root, Folders: []Folder{}, ScannedAt: time.Now()}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == TrashDirName {
			continue
		}
		containers := usage[e.Name()]
		report.Folders = append(report.Folders, Folder{
			Name:       e.Name(),
			Containers: containers,
			Orphan:     len(containers) == 0,
		})
	}
	report.recount()
	return report, nil
}

func (s *Scanner) folderPaths(r *Report) []string {
	paths := make([]string, len(r.Folders))
	for i, f := range r.Folders {
		paths[i] = filepath.Join(s.Root, f.Name)
	}
	return paths
}

// Delete moves the given top-level folders to the trash. Container usage is
// re-checked first so a folder that became used since the last scan is never removed.
func (s *Scanner) Delete(ctx context.Context, names []string) []Result {
	results := make([]Result, len(names))
	usage, usageErr := s.folderUsage(ctx)
	for i, name := range names {
		results[i].Name = name
		err := usageErr
		if err == nil {
			err = s.deleteOne(name, usage)
		}
		if err != nil {
			results[i].Error = err.Error()
		}
	}
	return results
}

func (s *Scanner) deleteOne(name string, usage map[string][]string) error {
	if !validName(name) || name == TrashDirName {
		return fmt.Errorf("invalid folder name %q", name)
	}
	if containers := usage[name]; len(containers) > 0 {
		return fmt.Errorf("still used by %s", strings.Join(containers, ", "))
	}
	p := filepath.Join(s.Root, name)
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", name)
	}
	return s.moveToTrash(name)
}

// validName reports whether name is a plain entry name, not a path.
func validName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

// folderUsage maps each top-level folder name to the containers that
// bind-mount it or something inside it.
func (s *Scanner) folderUsage(ctx context.Context) (map[string][]string, error) {
	containers, err := s.Docker.Containers(ctx)
	if err != nil {
		return nil, err
	}

	roots := s.roots()
	usage := make(map[string][]string)
	for _, c := range containers {
		seen := make(map[string]bool)
		for _, src := range c.Mounts {
			name, ok := topLevelFolder(src, roots)
			if ok && !seen[name] {
				seen[name] = true
				usage[name] = append(usage[name], c.Name)
			}
		}
	}
	for _, names := range usage {
		slices.Sort(names)
	}
	return usage, nil
}

// roots returns every path under which a mount source refers to Root.
func (s *Scanner) roots() []string {
	candidates := append([]string{s.Root}, s.Aliases...)
	if real, err := filepath.EvalSymlinks(s.Root); err == nil {
		candidates = append(candidates, real)
	}
	roots := make([]string, 0, len(candidates))
	for _, r := range candidates {
		r = path.Clean(filepath.ToSlash(r))
		if !slices.Contains(roots, r) {
			roots = append(roots, r)
		}
	}
	return roots
}

// topLevelFolder returns the first path component of src below one of roots.
// A mount of a root itself does not mark any folder as used.
func topLevelFolder(src string, roots []string) (string, bool) {
	src = path.Clean(filepath.ToSlash(src))
	for _, root := range roots {
		rest, ok := strings.CutPrefix(src, strings.TrimSuffix(root, "/")+"/")
		if !ok || rest == "" {
			continue
		}
		name, _, _ := strings.Cut(rest, "/")
		return name, true
	}
	return "", false
}

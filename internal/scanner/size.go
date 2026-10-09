package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"runtime"
	"sync"
)

// dirSizes computes the size of each directory concurrently. If onDone is not
// nil, it is called (from any goroutine) as each size becomes known.
func dirSizes(ctx context.Context, paths []string, onDone func(i int, size int64)) []int64 {
	sizes := make([]int64, len(paths))
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sizes[i] = dirSize(ctx, p)
			if onDone != nil {
				onDone(i, sizes[i])
			}
		}()
	}
	wg.Wait()
	return sizes
}

// dirSize returns the apparent size of the regular files under root.
// Unreadable entries are skipped rather than failing the whole scan.
func dirSize(ctx context.Context, root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || !d.Type().IsRegular() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

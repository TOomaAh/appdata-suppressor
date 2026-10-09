package scanner

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"
)

// Job runs scans in the background, one at a time, and keeps the latest report
// so the UI gets results instantly and sees sizes fill in while a scan runs.
type Job struct {
	sc  *Scanner
	ctx context.Context // canceled on shutdown

	mu      sync.Mutex
	running bool
	report  *Report // latest report, partial while running
	err     string  // error of the last scan
	done    int     // folders whose size is known in the running scan
	total   int
	idle    chan struct{} // closed when the running scan ends
}

// Status is a snapshot of a Job, safe to serialize.
type Status struct {
	Running bool    `json:"running"`
	Done    int     `json:"done"`
	Total   int     `json:"total"`
	Error   string  `json:"error,omitempty"`
	Report  *Report `json:"report"`
}

// NewJob returns a Job whose scans stop when ctx is canceled.
func NewJob(ctx context.Context, sc *Scanner) *Job {
	return &Job{sc: sc, ctx: ctx}
}

// Start launches a scan unless one is already running.
func (j *Job) Start() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.running {
		return
	}
	j.running = true
	j.err = ""
	j.done, j.total = 0, 0
	j.idle = make(chan struct{})
	go j.run()
}

// Status returns a copy of the job state.
func (j *Job) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	st := Status{Running: j.running, Done: j.done, Total: j.total, Error: j.err}
	if j.report != nil {
		r := *j.report
		r.Folders = slices.Clone(r.Folders)
		st.Report = &r
	}
	return st
}

// Forget drops folders from the cached report, e.g. after moving them to the
// trash, so the report stays accurate without a full rescan.
func (j *Job) Forget(names []string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.report == nil {
		return
	}
	j.report.Folders = slices.DeleteFunc(j.report.Folders, func(f Folder) bool {
		return slices.Contains(names, f.Name)
	})
	j.report.recount()
}

// Wait blocks until no scan is running.
func (j *Job) Wait() {
	j.mu.Lock()
	idle := j.idle
	j.mu.Unlock()
	if idle != nil {
		<-idle
	}
}

func (j *Job) run() {
	start := time.Now()
	report, err := j.sc.listFolders(j.ctx)
	if err != nil {
		j.finish(err)
		return
	}

	// Show the folder list right away, with the previous sizes as placeholders.
	j.mu.Lock()
	previous := map[string]int64{}
	if j.report != nil {
		for _, f := range j.report.Folders {
			previous[f.Name] = f.Size
		}
	}
	for i := range report.Folders {
		f := &report.Folders[i]
		f.Size = previous[f.Name]
		f.SizePending = true
	}
	report.recount()
	j.report = report
	j.total = len(report.Folders)
	paths := j.sc.folderPaths(report)
	names := make([]string, len(report.Folders))
	for i, f := range report.Folders {
		names[i] = f.Name
	}
	j.mu.Unlock()

	dirSizes(j.ctx, paths, func(i int, size int64) {
		j.mu.Lock()
		defer j.mu.Unlock()
		j.done++
		// Look up by name: Forget may have removed folders meanwhile.
		for k := range j.report.Folders {
			if f := &j.report.Folders[k]; f.Name == names[i] {
				f.Size = size
				f.SizePending = false
				j.report.recount()
				break
			}
		}
	})

	log.Printf("scan finished: %d folders in %s", len(names), time.Since(start).Round(time.Millisecond))
	j.finish(j.ctx.Err())
}

func (j *Job) finish(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.running = false
	close(j.idle)
	if err != nil {
		j.err = err.Error()
		log.Printf("scan failed: %v", err)
	}
}

package scanner

import (
	"context"
	"testing"
)

func TestJob(t *testing.T) {
	s := setup(t, map[string][]string{"plex": {"plex"}})
	job := NewJob(context.Background(), s)

	if st := job.Status(); st.Running || st.Report != nil {
		t.Fatalf("new job status = %+v; want idle without report", st)
	}

	job.Start()
	job.Start() // no-op while running
	job.Wait()

	st := job.Status()
	if st.Running || st.Error != "" || st.Done != 3 || st.Total != 3 {
		t.Fatalf("status = %+v; want finished 3/3 without error", st)
	}
	r := st.Report
	if r.TotalSize != 300 || r.OrphanCount != 2 || r.OrphanSize != 200 {
		t.Fatalf("report totals = %d/%d/%d; want 300/2/200", r.TotalSize, r.OrphanCount, r.OrphanSize)
	}
	for _, f := range r.Folders {
		if f.SizePending || f.Size != 100 {
			t.Errorf("folder %s: size %d pending %v; want 100, done", f.Name, f.Size, f.SizePending)
		}
	}

	// Status returns a copy: mutating it must not affect the job.
	r.Folders[0].Name = "mutated"

	job.Forget([]string{"old"})
	r = job.Status().Report
	if len(r.Folders) != 2 || r.OrphanCount != 1 || r.TotalSize != 200 {
		t.Errorf("after Forget: %d folders, %d orphans, %d bytes; want 2, 1, 200", len(r.Folders), r.OrphanCount, r.TotalSize)
	}
	for _, f := range r.Folders {
		if f.Name == "mutated" {
			t.Error("Status leaked internal state")
		}
	}

	// A rescan keeps working after Forget and finds the same folders again.
	job.Start()
	job.Wait()
	if n := len(job.Status().Report.Folders); n != 3 {
		t.Errorf("rescan found %d folders, want 3", n)
	}
}

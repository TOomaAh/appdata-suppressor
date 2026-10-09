// Package server exposes the scanner over HTTP and serves the web UI.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"appdata-suppressor/internal/auth"
	"appdata-suppressor/internal/scanner"
	"appdata-suppressor/web"
)

const maxBodyBytes = 1 << 20

type Options struct {
	Scanner   *scanner.Scanner
	Job       *scanner.Job
	Auth      *auth.Auth // nil disables authentication
	TrashDays int        // only reported to the UI
}

// New returns the HTTP handler for the API and the static UI.
func New(o Options) http.Handler {
	sc, job, a := o.Scanner, o.Job, o.Auth
	mux := http.NewServeMux()

	if a != nil {
		mux.HandleFunc("POST /api/login", a.HandleLogin)
		mux.HandleFunc("POST /api/logout", a.HandleLogout)
	}

	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"auth": a != nil})
	})

	// The scan runs in the background: POST starts it, GET polls its progress
	// and returns the latest (possibly partial) report.
	mux.HandleFunc("GET /api/scan", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, job.Status())
	})

	mux.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		job.Start()
		writeJSON(w, http.StatusAccepted, job.Status())
	})

	mux.HandleFunc("POST /api/delete", batch("names", func(r *http.Request, names []string) []scanner.Result {
		results := logResults("moved to trash", sc.Delete(r.Context(), names))
		job.Forget(succeeded(results))
		return results
	}))

	mux.HandleFunc("GET /api/trash", func(w http.ResponseWriter, r *http.Request) {
		items, err := sc.Trash(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "retention_days": o.TrashDays})
	})

	mux.HandleFunc("POST /api/trash/restore", batch("ids", func(_ *http.Request, ids []string) []scanner.Result {
		results := logResults("restored", sc.Restore(ids))
		if len(succeeded(results)) > 0 {
			job.Start() // restored folders need their usage and size
		}
		return results
	}))

	mux.HandleFunc("POST /api/trash/purge", batch("ids", func(_ *http.Request, ids []string) []scanner.Result {
		return logResults("permanently deleted", sc.Purge(ids))
	}))

	mux.Handle("GET /", http.FileServerFS(web.Static))

	if a == nil {
		return mux
	}
	return a.Middleware(mux)
}

// batch handles a POST whose JSON body is {key: [...]} and replies with the
// per-item results of op.
func batch(key string, op func(*http.Request, []string) []scanner.Result) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.IsJSON(r) {
			writeError(w, http.StatusUnsupportedMediaType, "expected application/json")
			return
		}
		var req map[string][]string
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req[key]) == 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: expected {%q: [...]}", key))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": op(r, req[key])})
	}
}

func logResults(action string, results []scanner.Result) []scanner.Result {
	for _, res := range results {
		if res.Error == "" {
			log.Printf("%s: %s", action, res.Name)
		} else {
			log.Printf("failed (%s) %s: %s", action, res.Name, res.Error)
		}
	}
	return results
}

func succeeded(results []scanner.Result) []string {
	var names []string
	for _, res := range results {
		if res.Error == "" {
			names = append(names, res.Name)
		}
	}
	return names
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

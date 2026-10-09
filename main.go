// Command appdata-suppressor serves a web UI listing appdata folders that no
// Docker container uses anymore, and lets you delete them.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"appdata-suppressor/internal/auth"
	"appdata-suppressor/internal/docker"
	"appdata-suppressor/internal/scanner"
	"appdata-suppressor/internal/server"
)

// stringList is a repeatable string flag.
type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

func main() {
	root := flag.String("path", "/mnt/nvme/appdata", "path to the appdata directory")
	host := flag.String("host", "", "interface to listen on (empty = all)")
	port := flag.String("port", "8765", "port to listen on")
	var aliases stringList
	flag.Var(&aliases, "alias", "other path that points to -path, e.g. /mnt/user/appdata (repeatable)")
	user := flag.String("user", envOr("APPDATA_USER", "admin"), "login user name (env APPDATA_USER)")
	password := flag.String("password", "", "login password; prefer env APPDATA_PASSWORD, flags are visible in ps")
	sessionTTL := flag.Duration("session-ttl", 7*24*time.Hour, "how long a login stays valid")
	trashDays := flag.Int("trash-days", 7, "days before trashed folders are permanently deleted (0 = never)")
	noAuth := flag.Bool("no-auth", false, "disable authentication (only behind a reverse proxy that handles it)")
	flag.Parse()

	if *password == "" {
		*password = os.Getenv("APPDATA_PASSWORD")
	}
	var authn *auth.Auth
	switch {
	case *noAuth:
		log.Print("⚠️  authentication disabled (-no-auth)")
	case *password == "":
		log.Fatal("no password set: use env APPDATA_PASSWORD (or -password), or -no-auth to disable authentication")
	default:
		authn = auth.New(*user, *password, *sessionTTL)
	}

	if info, err := os.Stat(*root); err != nil || !info.IsDir() {
		log.Fatalf("appdata path %q is not a readable directory", *root)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sc := &scanner.Scanner{Root: *root, Aliases: aliases, Docker: docker.CLI{}}
	job := scanner.NewJob(ctx, sc)
	job.Start() // so the first page load already has results
	srv := &http.Server{
		Addr:              net.JoinHostPort(*host, *port),
		Handler:           server.New(server.Options{Scanner: sc, Job: job, Auth: authn, TrashDays: *trashDays}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	if *trashDays > 0 {
		go purgeTrashLoop(ctx, sc, time.Duration(*trashDays)*24*time.Hour)
	}

	go func() {
		log.Printf("🐳 Appdata Cleaner running on http://localhost:%s", *port)
		log.Printf("📂 Scanning: %s", *root)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// purgeTrashLoop empties expired trash entries at startup and then hourly.
func purgeTrashLoop(ctx context.Context, sc *scanner.Scanner, maxAge time.Duration) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		purged, err := sc.PurgeExpired(ctx, maxAge)
		for _, id := range purged {
			log.Printf("trash expired, permanently deleted: %s", id)
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("trash purge: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

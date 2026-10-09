package docker

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const containersJSON = `[
	{"Names": ["/plex"], "Mounts": [
		{"Type": "bind", "Source": "/mnt/appdata/plex"},
		{"Type": "volume", "Source": "/var/lib/docker/volumes/x/_data"},
		{"Type": "bind", "Source": "/mnt/media"}
	]},
	{"Names": ["/idle"], "Mounts": []}
]`

func fakeDaemon() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" || r.URL.Query().Get("all") != "1" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message": "page not found"}`))
			return
		}
		w.Write([]byte(containersJSON))
	})
}

func checkContainers(t *testing.T, c *Client) {
	t.Helper()
	got, err := c.Containers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "plex" || got[1].Name != "idle" {
		t.Fatalf("containers = %+v", got)
	}
	if want := []string{"/mnt/appdata/plex", "/mnt/media"}; !slices.Equal(got[0].Mounts, want) {
		t.Errorf("plex mounts = %v; want only bind mounts %v", got[0].Mounts, want)
	}
}

func TestTCP(t *testing.T) {
	srv := httptest.NewServer(fakeDaemon())
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	checkContainers(t, c)
}

func TestUnixSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix:// URLs take Linux socket paths")
	}
	socket := filepath.Join(t.TempDir(), "docker.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	srv := &httptest.Server{Listener: l, Config: &http.Server{Handler: fakeDaemon()}}
	srv.Start()
	defer srv.Close()

	c, err := New("unix://" + socket)
	if err != nil {
		t.Fatal(err)
	}
	checkContainers(t, c)
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "daemon is sad"}`))
	}))
	defer srv.Close()
	c, _ := New("tcp://" + srv.Listener.Addr().String())
	_, err := c.Containers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "daemon is sad") {
		t.Errorf("err = %v; want the daemon message", err)
	}
}

func TestUnsupportedHost(t *testing.T) {
	if _, err := New("npipe:////./pipe/docker_engine"); err == nil {
		t.Error("npipe host should be rejected")
	}
}

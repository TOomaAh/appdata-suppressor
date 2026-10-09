// Package docker lists containers and their bind mounts through the Docker
// Engine API, over the daemon's unix socket or TCP.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultHost is the daemon address used when DOCKER_HOST is not set.
const DefaultHost = "unix:///var/run/docker.sock"

// Container is the subset of the container list the scanner needs.
type Container struct {
	Name   string
	Mounts []string // sources of bind mounts
}

// Client is a minimal Docker Engine API client.
type Client struct {
	http *http.Client
	base string // URL prefix of API requests
}

// New returns a client for host, given as unix:///path/to/docker.sock or
// tcp://host:port (plain HTTP; TLS is not supported).
func New(host string) (*Client, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("invalid docker host %q: %w", host, err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	c := &Client{http: &http.Client{Transport: transport, Timeout: 30 * time.Second}}
	switch u.Scheme {
	case "unix":
		socket := u.Path
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		}
		c.base = "http://docker" // host is ignored, every request goes to the socket
	case "tcp":
		c.base = "http://" + u.Host
	default:
		return nil, fmt.Errorf("unsupported docker host %q: use unix:// or tcp://", host)
	}
	return c, nil
}

// Containers returns every container (running or not) with its bind mount sources.
func (c *Client) Containers(ctx context.Context) ([]Container, error) {
	var raw []struct {
		Names  []string
		Mounts []struct {
			Type   string
			Source string
		}
	}
	if err := c.get(ctx, "/containers/json?all=1", &raw); err != nil {
		return nil, err
	}

	containers := make([]Container, 0, len(raw))
	for _, r := range raw {
		var c Container
		if len(r.Names) > 0 {
			c.Name = strings.TrimPrefix(r.Names[0], "/")
		}
		for _, m := range r.Mounts {
			if m.Type == "bind" {
				c.Mounts = append(c.Mounts, m.Source)
			}
		}
		containers = append(containers, c)
	}
	return containers, nil
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr struct{ Message string }
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if json.Unmarshal(body, &apiErr) != nil || apiErr.Message == "" {
			apiErr.Message = strings.TrimSpace(string(body))
		}
		return fmt.Errorf("docker API %s: %s: %s", path, resp.Status, apiErr.Message)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("docker API %s: decode response: %w", path, err)
	}
	return nil
}

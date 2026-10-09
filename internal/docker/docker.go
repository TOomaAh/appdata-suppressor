// Package docker lists containers and their bind mounts through the docker CLI.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Container is the subset of `docker inspect` output the scanner needs.
type Container struct {
	Name   string
	Mounts []string // sources of bind mounts
}

// CLI talks to the Docker daemon by shelling out to the docker binary.
type CLI struct {
	Binary string // defaults to "docker"
}

// Containers returns every container (running or not) with its bind mount sources.
func (c CLI) Containers(ctx context.Context) ([]Container, error) {
	out, err := c.run(ctx, "ps", "--all", "--quiet", "--no-trunc")
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil, nil
	}

	// A container removed between `ps` and `inspect` makes inspect exit non-zero
	// while still printing the others, so only fail if nothing usable came back.
	out, inspectErr := c.run(ctx, append([]string{"inspect"}, ids...)...)
	var raw []struct {
		Name   string
		Mounts []struct {
			Type   string
			Source string
		}
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		if inspectErr != nil {
			return nil, inspectErr
		}
		return nil, fmt.Errorf("parse docker inspect output: %w", err)
	}

	containers := make([]Container, 0, len(raw))
	for _, r := range raw {
		c := Container{Name: strings.TrimPrefix(r.Name, "/")}
		for _, m := range r.Mounts {
			if m.Type == "bind" {
				c.Mounts = append(c.Mounts, m.Source)
			}
		}
		containers = append(containers, c)
	}
	return containers, nil
}

func (c CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	bin := c.Binary
	if bin == "" {
		bin = "docker"
	}
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return out, fmt.Errorf("docker %s: %w", args[0], err)
	}
	return out, nil
}

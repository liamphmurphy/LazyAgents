package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

var (
	// ServiceReadyTimeout bounds how long a service may take to pass its
	// healthcheck (or start running, without one).
	ServiceReadyTimeout = 2 * time.Minute
	// servicePullTimeout bounds pulling a missing service image.
	servicePullTimeout = 5 * time.Minute
	servicePoll        = 500 * time.Millisecond
)

func serviceNetworkName(key string) string { return "lazyagents-svc-" + key }

func serviceContainerName(key, service string) string { return "lazyagents-svc-" + key + "-" + service }

func (d *Docker) StartServices(ctx context.Context, runID, key string, services []agentdef.Service) (string, func(), error) {
	network := serviceNetworkName(key)
	var started []string
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		// Its own context: this runs when the turn ends, shutdown included.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, id := range started {
			_, _ = d.do(ctx, "DELETE", "/containers/"+id+"?force=1", nil, "", nil)
		}
		_, _ = d.do(ctx, "DELETE", "/networks/"+url.PathEscape(network), nil, "", nil)
	}
	// Internal: services can't reach the internet or the host, only the turn
	// (which is also on the runner's network) can reach them.
	_, err := d.do(ctx, "POST", "/networks/create", jsonBody(map[string]any{
		"Name": network, "Internal": true, "Labels": d.labels(runID),
	}), "application/json", nil)
	if isStatus(err, 409) { // left by a turn that died; reuse it
		err = nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("create service network: %w", err)
	}
	for _, s := range services {
		id, err := d.startService(ctx, runID, key, network, s)
		if id != "" {
			started = append(started, id)
		}
		if err != nil {
			stop()
			return "", nil, fmt.Errorf("service %s: %w", s.Name, err)
		}
	}
	for i, s := range services {
		if err := d.waitReady(ctx, started[i], s); err != nil {
			stop()
			return "", nil, fmt.Errorf("service %s: %w", s.Name, err)
		}
	}
	return network, stop, nil
}

// serviceBody is the create request for one service: the turn's sandbox
// with Docker's default capabilities, on the service network only, reachable
// there by its name.
func (d *Docker) serviceBody(runID, network string, s agentdef.Service) map[string]any {
	env := make([]string, 0, len(s.Env))
	for k, v := range s.Env {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	mounts := []map[string]any{}
	for _, p := range s.Tmpfs {
		// World-writable like /tmp, so an unprivileged service user can own
		// what it creates inside.
		mounts = append(mounts, map[string]any{"Type": "tmpfs", "Target": p, "TmpfsOptions": map[string]any{"Mode": 0o1777}})
	}
	host := d.hostConfig(mounts)
	host["NetworkMode"] = network
	delete(host, "ExtraHosts") // nothing on the host is a service's business
	// Docker's default capabilities, unlike the turn: official database
	// images start as root and switch to their own user, and a recipe
	// inferred from CI names no user. The line held is the container's own
	// boundary, as for what turns start through their Docker proxy.
	delete(host, "CapDrop")
	body := map[string]any{
		"Image": s.Image, "Env": env, "Labels": d.labels(runID),
		"HostConfig": host,
		"NetworkingConfig": map[string]any{"EndpointsConfig": map[string]any{
			network: map[string]any{"Aliases": []string{s.Name}},
		}},
	}
	if len(s.Command) > 0 {
		body["Cmd"] = s.Command
	}
	if s.User != "" {
		body["User"] = s.User
	}
	if len(s.Healthcheck) > 0 {
		body["Healthcheck"] = map[string]any{
			"Test": s.Healthcheck, "Interval": int64(time.Second), "Timeout": int64(5 * time.Second),
			"Retries": 3, "StartPeriod": int64(ServiceReadyTimeout),
		}
	}
	return body
}

func (d *Docker) startService(ctx context.Context, runID, key, network string, s agentdef.Service) (string, error) {
	name := url.QueryEscape(serviceContainerName(key, s.Name))
	body := d.serviceBody(runID, network, s)
	var created struct{ ID string }
	create := func() error {
		_, err := d.do(ctx, "POST", "/containers/create?name="+name, jsonBody(body), "application/json", &created)
		return err
	}
	err := create()
	if isStatus(err, 409) { // a stale container with this name
		_, _ = d.do(ctx, "DELETE", "/containers/"+name+"?force=1", nil, "", nil)
		err = create()
	}
	if isStatus(err, 404) { // the image isn't here yet
		if err = d.pull(ctx, s.Image); err == nil {
			err = create()
		}
	}
	if err != nil {
		return "", err
	}
	if _, err := d.do(ctx, "POST", "/containers/"+created.ID+"/start", nil, "", nil); err != nil {
		return created.ID, err
	}
	return created.ID, nil
}

// pull fetches an image. The Engine streams progress as JSON lines and
// reports a failed pull in the stream, not the status code.
func (d *Docker) pull(ctx context.Context, image string) error {
	ctx, cancel := context.WithTimeout(ctx, servicePullTimeout)
	defer cancel()
	ref, tag := image, "latest"
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		ref, tag = image[:i], image[i+1:]
	}
	if strings.Contains(image, "@") {
		ref, tag = image, ""
	}
	q := url.Values{"fromImage": {ref}}
	if tag != "" {
		q.Set("tag", tag)
	}
	res, err := d.do(ctx, "POST", "/images/create?"+q.Encode(), nil, "", nil)
	if err != nil {
		return fmt.Errorf("pull %s: %w", image, err)
	}
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var msg struct{ Error string }
		if json.Unmarshal(sc.Bytes(), &msg) == nil && msg.Error != "" {
			return fmt.Errorf("pull %s: %s", image, msg.Error)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("pull %s: %w", image, err)
	}
	return nil
}

// waitReady waits for a service's healthcheck to pass, or, without one, for
// it to be running. A service that exits fails with the end of its logs.
func (d *Docker) waitReady(ctx context.Context, id string, s agentdef.Service) error {
	ctx, cancel := context.WithTimeout(ctx, ServiceReadyTimeout)
	defer cancel()
	notReady := func() error {
		return fmt.Errorf("not ready after %s:\n%s", ServiceReadyTimeout, d.logTail(id))
	}
	for {
		var info struct {
			State struct {
				Status   string
				ExitCode int
				Health   *struct{ Status string }
			}
		}
		if _, err := d.do(ctx, "GET", "/containers/"+id+"/json", nil, "", &info); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return notReady() // the deadline fell during the inspect
			}
			return err
		}
		switch {
		case info.State.Status == "exited" || info.State.Status == "dead":
			return fmt.Errorf("exited with code %d:\n%s", info.State.ExitCode, d.logTail(id))
		case len(s.Healthcheck) == 0 && info.State.Status == "running":
			return nil
		case info.State.Health != nil && info.State.Health.Status == "healthy":
			return nil
		case info.State.Health != nil && info.State.Health.Status == "unhealthy":
			return fmt.Errorf("its healthcheck failed:\n%s", d.logTail(id))
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return notReady()
			}
			return ctx.Err()
		case <-time.After(servicePoll):
		}
	}
}

// logTail is the end of a container's output, for a failure message.
func (d *Docker) logTail(id string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := d.do(ctx, "GET", "/containers/"+id+"/logs?stdout=1&stderr=1&tail=20", nil, "", nil)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var out bytes.Buffer
	_ = demux(io.LimitReader(res.Body, 64<<10), &out, &out)
	return strings.TrimSpace(out.String())
}

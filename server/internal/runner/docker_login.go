package runner

import (
	"archive/tar"
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"
)

// maxLoginFile bounds a file read back out of a sign-in container.
const maxLoginFile = 64 << 10

// StartLogin runs a provider's sign-in CLI in a container of its own: the
// agent image with the turn sandbox, but no volumes, so nothing it writes
// outlives Close except what the runner reads back with ReadFile.
func (d *Docker) StartLogin(ctx context.Context, spec LoginSpec) (LoginProcess, error) {
	env := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	body := map[string]any{
		"Image": d.Image, "Cmd": spec.Argv, "Env": env, "WorkingDir": homePath,
		"Labels": d.labels(spec.Label),
		"Tty":    spec.TTY, "OpenStdin": spec.TTY, "AttachStdin": spec.TTY, "AttachStdout": true, "AttachStderr": true,
		"HostConfig": d.hostConfig([]map[string]any{}),
	}
	var created struct{ ID string }
	name := url.QueryEscape(spec.Name)
	if _, err := d.do(ctx, "POST", "/containers/create?name="+name, jsonBody(body), "application/json", &created); err != nil {
		if isStatus(err, 409) {
			_, _ = d.do(ctx, "DELETE", "/containers/"+name+"?force=1", nil, "", nil)
			_, err = d.do(ctx, "POST", "/containers/create?name="+name, jsonBody(body), "application/json", &created)
		}
		if err != nil {
			return nil, err
		}
	}
	p := &dockerLogin{d: d, id: created.ID}
	stream, err := d.attachLogin(created.ID, spec.TTY)
	if err != nil {
		p.Close()
		return nil, err
	}
	p.stream = stream
	if _, err := d.do(ctx, "POST", "/containers/"+created.ID+"/start", nil, "", nil); err != nil {
		p.Close()
		return nil, err
	}
	if spec.TTY {
		// A terminal is one raw stream, both ways.
		p.out, p.in = stream, stream
		return p, nil
	}
	r, w := io.Pipe()
	p.out = r
	go func() { w.CloseWithError(demux(stream, w, w)) }()
	return p, nil
}

// attachLogin attaches before start so no output is lost. With a terminal it
// upgrades the connection so the runner can type into it.
func (d *Docker) attachLogin(id string, tty bool) (io.ReadWriteCloser, error) {
	q := "stream=1&stdout=1&stderr=1"
	if tty {
		q += "&stdin=1"
	}
	req, err := http.NewRequest("POST", dockerAPI+"/containers/"+id+"/attach?"+q, nil)
	if err != nil {
		return nil, err
	}
	if tty {
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "tcp")
	}
	res, err := d.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker attach: %w", err)
	}
	if tty {
		rw, ok := res.Body.(io.ReadWriteCloser)
		if res.StatusCode != http.StatusSwitchingProtocols || !ok {
			res.Body.Close()
			return nil, fmt.Errorf("docker attach: no interactive stream (status %d)", res.StatusCode)
		}
		return rw, nil
	}
	if res.StatusCode >= 300 {
		res.Body.Close()
		return nil, &dockerError{Status: res.StatusCode, Msg: "attach failed"}
	}
	return readOnly{res.Body}, nil
}

type readOnly struct{ io.ReadCloser }

func (readOnly) Write([]byte) (int, error) { return 0, fmt.Errorf("docker: not an interactive stream") }

type dockerLogin struct {
	d      *Docker
	id     string
	stream io.ReadWriteCloser
	out    io.Reader
	in     io.Writer
	once   sync.Once
}

func (p *dockerLogin) Output() io.Reader { return p.out }
func (p *dockerLogin) Input() io.Writer  { return p.in }

func (p *dockerLogin) Wait() (int, error) {
	var res struct{ StatusCode int }
	if _, err := p.d.do(context.Background(), "POST", "/containers/"+p.id+"/wait", nil, "", &res); err != nil {
		return -1, err
	}
	return res.StatusCode, nil
}

func (p *dockerLogin) Kill() error {
	_, err := p.d.do(context.Background(), "POST", "/containers/"+p.id+"/kill?signal=SIGKILL", nil, "", nil)
	if isStatus(err, 409) {
		return nil
	}
	return err
}

// ReadFile reads one file from the container, which must not be removed yet.
func (p *dockerLogin) ReadFile(ctx context.Context, file string) ([]byte, error) {
	res, err := p.d.do(ctx, "GET", "/containers/"+p.id+"/archive?path="+url.QueryEscape(file), nil, "", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	tr := tar.NewReader(bufio.NewReader(res.Body))
	if _, err := tr.Next(); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(tr, maxLoginFile+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxLoginFile {
		return nil, fmt.Errorf("%s is larger than %d bytes", file, maxLoginFile)
	}
	return b, nil
}

func (p *dockerLogin) Close() {
	p.once.Do(func() {
		if p.stream != nil {
			p.stream.Close()
		}
		_, _ = p.d.do(context.Background(), "DELETE", "/containers/"+p.id+"?force=1", nil, "", nil)
	})
}

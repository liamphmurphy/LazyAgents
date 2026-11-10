package runner_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/auth"
	"github.com/liamphmurphy/vesper/server/internal/playground"
	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// fakePlaygrounds runs a playground's processes as one test server, standing
// in for the containers.
type fakePlaygrounds struct {
	runner.Backend
	app *httptest.Server

	mu      sync.Mutex
	started map[uuid.UUID]int
	stopped map[uuid.UUID]int
	fail    error
}

func (f *fakePlaygrounds) StartPlayground(_ context.Context, pg runner.PlaygroundRun, step func(string)) (map[string]string, error) {
	step("Starting " + pg.Spec.Processes[0].Name)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, f.fail
	}
	f.started[pg.ID]++
	addrs := map[string]string{}
	for _, p := range pg.Spec.Processes {
		addrs[p.Name] = strings.TrimPrefix(f.app.URL, "http://")
	}
	return addrs, nil
}

func (f *fakePlaygrounds) StopPlayground(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped[id]++
	return nil
}

func (f *fakePlaygrounds) RunningPlaygrounds(context.Context) ([]uuid.UUID, error) { return nil, nil }
func (f *fakePlaygrounds) StackName() string                                       { return "test" }
func (f *fakePlaygrounds) PlaygroundNetwork(id uuid.UUID) string                   { return "play-" + id.String() }
func (f *fakePlaygrounds) ReadFile(context.Context, uuid.UUID, string) ([]byte, error) {
	return nil, io.EOF
}
func (f *fakePlaygrounds) Logs(context.Context, uuid.UUID, int, bool, func(playground.LogLine) error) error {
	return nil
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type playgroundRig struct {
	env
	fake    *fakePlaygrounds
	ports   []int
	session *http.Cookie
	cancel  context.CancelFunc
}

func setupPlaygrounds(t *testing.T, ports int) playgroundRig {
	e := setup(t)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "app saw cookie="+r.Header.Get("Cookie"))
	}))
	t.Cleanup(app.Close)
	fake := &fakePlaygrounds{Backend: e.runner.Backend, app: app, started: map[uuid.UUID]int{}, stopped: map[uuid.UUID]int{}}
	e.runner.Backend = fake
	rig := playgroundRig{env: e, fake: fake}
	for range ports {
		rig.ports = append(rig.ports, freePort(t))
	}
	e.runner.Cfg.Playgrounds = runner.PlaygroundConfig{Ports: rig.ports, Origins: []string{"http://localhost:3000"}}
	ctx, cancel := context.WithCancel(e.ctx)
	t.Cleanup(cancel)
	rig.cancel = cancel
	e.runner.ServePlaygrounds(ctx)
	code, err := auth.NewSignInCode(e.ctx, e.q)
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.RedeemSignInCode(e.ctx, e.q, code)
	if err != nil {
		t.Fatal(err)
	}
	rig.session = &http.Cookie{Name: auth.SessionCookie, Value: token}
	return rig
}

// startPlayground gives a chat with a workspace a playground to start.
func (rig playgroundRig) startPlayground() store.Playground {
	rig.t.Helper()
	chat := rig.startChat("Show me the app")
	rig.until("chat reply", func() bool { return rig.chat(chat.ID).State == "idle" })
	run := rig.chatRun(chat.ID)
	spec, _ := json.Marshal(playground.Spec{Processes: []playground.Process{{Name: "web", Command: "npm run dev", Port: 3000}}})
	pg, err := rig.q.StartChatPlayground(rig.ctx, store.StartChatPlaygroundParams{WorkspaceID: rig.ws, UserID: rig.me.ID, ChatID: chat.ID, RunID: &run.ID, Spec: spec})
	if err != nil {
		rig.t.Fatal(err)
	}
	return pg
}

func (rig playgroundRig) playground(chat uuid.UUID) store.Playground {
	rig.t.Helper()
	pg, err := rig.q.GetChatPlayground(rig.ctx, store.GetChatPlaygroundParams{ChatID: chat, WorkspaceID: rig.ws})
	if err != nil {
		rig.t.Fatal(err)
	}
	return pg
}

func (rig playgroundRig) get(port int, cookie *http.Cookie) (int, string) {
	rig.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		rig.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestPlaygroundStartsServesAndStops(t *testing.T) {
	rig := setupPlaygrounds(t, 1)
	pg := rig.startPlayground()
	rig.until("playground running", func() bool { return rig.playground(pg.ChatID).State == "running" })
	running := rig.playground(pg.ChatID)
	if running.Port == nil || int(*running.Port) != rig.ports[0] || running.StartedAt == nil || running.DataResetAt == nil {
		t.Fatalf("running playground %+v", running)
	}

	if code, _ := rig.get(rig.ports[0], nil); code != http.StatusUnauthorized {
		t.Errorf("without a Vesper session = %d, want 401", code)
	}
	code, body := rig.get(rig.ports[0], rig.session)
	if code != http.StatusOK || !strings.HasPrefix(body, "app saw cookie=") || strings.Contains(body, rig.session.Value) {
		t.Fatalf("signed in = %d %q: want the app, without Vesper's session", code, body)
	}

	if _, err := rig.q.RequestPlaygroundAction(rig.ctx, store.RequestPlaygroundActionParams{Action: refOf("stop"), ChatID: pg.ChatID, WorkspaceID: rig.ws}); err != nil {
		t.Fatal(err)
	}
	rig.until("playground stopped", func() bool { return rig.playground(pg.ChatID).State == "stopped" })
	if stopped := rig.playground(pg.ChatID); stopped.Port != nil || stopped.StopReason != "Stopped" || stopped.Action != nil {
		t.Fatalf("stopped playground %+v", stopped)
	}
	if code, _ := rig.get(rig.ports[0], rig.session); code != http.StatusServiceUnavailable {
		t.Errorf("after stopping = %d, want 503", code)
	}

	// The person starts it again with the same spec, on the freed port.
	if _, err := rig.q.RestartStoppedPlayground(rig.ctx, store.RestartStoppedPlaygroundParams{ChatID: pg.ChatID, WorkspaceID: rig.ws, RunID: pg.RunID}); err != nil {
		t.Fatal(err)
	}
	rig.until("playground running again", func() bool { return rig.playground(pg.ChatID).State == "running" })
	if code, _ := rig.get(rig.ports[0], rig.session); code != http.StatusOK {
		t.Errorf("after starting again = %d, want 200", code)
	}
}

func TestPlaygroundResetStartsFreshOnTheSamePort(t *testing.T) {
	rig := setupPlaygrounds(t, 1)
	pg := rig.startPlayground()
	rig.until("playground running", func() bool { return rig.playground(pg.ChatID).State == "running" })
	before := rig.playground(pg.ChatID)
	if _, err := rig.q.RequestPlaygroundAction(rig.ctx, store.RequestPlaygroundActionParams{Action: refOf("reset"), ChatID: pg.ChatID, WorkspaceID: rig.ws}); err != nil {
		t.Fatal(err)
	}
	rig.until("data reset", func() bool {
		p := rig.playground(pg.ChatID)
		return p.Action == nil && p.DataResetAt.After(*before.DataResetAt)
	})
	after := rig.playground(pg.ChatID)
	if after.State != "running" || *after.Port != *before.Port || !after.StartedAt.Equal(*before.StartedAt) {
		t.Fatalf("after reset %+v, before %+v", after, before)
	}
	rig.fake.mu.Lock()
	defer rig.fake.mu.Unlock()
	if rig.fake.started[pg.ID] != 2 {
		t.Errorf("started %d times, want 2", rig.fake.started[pg.ID])
	}
}

func TestPlaygroundFailureKeepsTheLogAndFreesThePort(t *testing.T) {
	rig := setupPlaygrounds(t, 1)
	rig.fake.fail = &runner.PlaygroundError{Msg: "setup exited with code 1", Log: "migrate: relation already exists"}
	pg := rig.startPlayground()
	rig.until("playground failed", func() bool { return rig.playground(pg.ChatID).State == "failed" })
	failed := rig.playground(pg.ChatID)
	if *failed.Error != "setup exited with code 1" || failed.ErrorLog != "migrate: relation already exists" || failed.Port != nil {
		t.Fatalf("failed playground %+v", failed)
	}

	// The port went back: another playground can start on it.
	rig.fake.mu.Lock()
	rig.fake.fail = nil
	rig.fake.mu.Unlock()
	other := rig.startPlayground()
	rig.until("another playground running", func() bool { return rig.playground(other.ChatID).State == "running" })
}

func TestPlaygroundWithoutAFreePortIsRefused(t *testing.T) {
	rig := setupPlaygrounds(t, 1)
	first := rig.startPlayground()
	rig.until("first running", func() bool { return rig.playground(first.ChatID).State == "running" })
	second := rig.startPlayground()
	rig.until("second refused", func() bool { return rig.playground(second.ChatID).State == "failed" })
	if msg := *rig.playground(second.ChatID).Error; !strings.Contains(msg, "in use (1)") {
		t.Fatalf("refusal %q", msg)
	}
}

func TestPlaygroundStopsWhenUnused(t *testing.T) {
	rig := setupPlaygrounds(t, 1)
	rig.runner.Cfg.Playgrounds.IdleTTL = time.Nanosecond
	pg := rig.startPlayground()
	rig.until("playground running", func() bool { return rig.playground(pg.ChatID).State == "running" })
	time.Sleep(time.Millisecond)
	rig.runner.Sweep(rig.ctx)
	if p := rig.playground(pg.ChatID); p.State != "stopped" || p.StopReason != "Stopped after 1 min unused" || p.LastUsedAt == nil {
		t.Fatalf("after an idle sweep %+v", p)
	}
}

func refOf[T any](v T) *T { return &v }

// While a chat's playground runs, the chat's turns can reach it through the
// gateway (PLAYGROUND_URL), admitted by its token.
func TestChatTurnsReachTheirRunningPlayground(t *testing.T) {
	rig := setupPlaygrounds(t, 1)
	t.Setenv("FAKE_HARNESS_MODE", "chat")
	p := rig.persona("Pair", true, false)
	chat, err := rig.runs.StartChat(rig.ctx, rig.ws, rig.me.ID, runs.ChatInput{Project: "PAY", Title: "App", PersonaID: &p.ID, Message: &runs.ChatMessageInput{Body: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	rig.until("first reply", func() bool { return rig.chat(chat.ID).State == "idle" })
	run := rig.chatRun(chat.ID)
	spec, _ := json.Marshal(playground.Spec{Processes: []playground.Process{{Name: "web", Command: "npm run dev", Port: 3000}}})
	if _, err := rig.q.StartChatPlayground(rig.ctx, store.StartChatPlaygroundParams{WorkspaceID: rig.ws, UserID: rig.me.ID, ChatID: chat.ID, RunID: &run.ID, Spec: spec}); err != nil {
		t.Fatal(err)
	}
	rig.until("playground running", func() bool { return rig.playground(chat.ID).State == "running" })

	reply := rig.postSh(chat.ID, `echo "url=$PLAYGROUND_URL len=${#PLAYGROUND_TOKEN} enter=$PLAYGROUND_ENTER_URL"`)
	want := "url=http://playground:" + strconv.Itoa(rig.ports[0])
	if !strings.Contains(reply, want) || !strings.Contains(reply, "len=48") {
		t.Fatalf("turn saw %q, want %s and a 48-character token", reply, want)
	}
	if !regexp.MustCompile(`enter=http://playground:\d+/__vesper/enter\?token=[0-9a-f]{48}`).MatchString(reply) {
		t.Errorf("turn saw no enter URL: %q", reply)
	}
}

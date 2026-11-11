package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/auth"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testdb"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

func postJSON(h http.Handler, path, body string, c *http.Cookie, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookie {
			return c
		}
	}
	return nil
}

func readStatus(t *testing.T, h http.Handler, c *http.Cookie) authStatus {
	t.Helper()
	rec := getWithCookie(h, AuthStatusPath, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", AuthStatusPath, rec.Code, rec.Body)
	}
	var s authStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

const ownerCreds = `{"login":"Liam","password":"correct horse"}`

// A fresh install is set up from the browser: no terminal, no sign-in link.
// The first browser claims it, and from then on the open window is shut.
func TestFirstRunSetupClaimsTheInstanceOnce(t *testing.T) {
	_, h := newSignedInServer(t)

	if got := readStatus(t, h, nil); got != (authStatus{}) {
		t.Fatalf("fresh install status = %+v, want unclaimed, no password, signed out", got)
	}

	rec := postJSON(h, AuthSetupPath, ownerCreds, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body)
	}
	cookie := sessionCookie(rec)
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("setup cookie = %+v, want an HttpOnly, SameSite=Strict session", cookie)
	}
	if rec := getWithCookie(h, "/api/v1/bootstrap", cookie); rec.Code != http.StatusOK {
		t.Fatalf("after setup: GET /bootstrap = %d", rec.Code)
	}
	if got := readStatus(t, h, cookie); got != (authStatus{Claimed: true, PasswordSet: true, SignedIn: true}) {
		t.Errorf("after setup status = %+v", got)
	}

	// Someone else arriving later can't take the instance over, signed in or not.
	if rec := postJSON(h, AuthSetupPath, `{"login":"mallory","password":"hunter2hunter2"}`, nil); rec.Code != http.StatusConflict {
		t.Errorf("second setup, signed out = %d, want 409", rec.Code)
	}
	if rec := postJSON(h, AuthSetupPath, `{"login":"mallory","password":"hunter2hunter2"}`, cookie); rec.Code != http.StatusConflict {
		t.Errorf("second setup, signed in = %d, want 409", rec.Code)
	}
	if rec := postJSON(h, AuthLoginPath, ownerCreds, nil); rec.Code != http.StatusOK {
		t.Errorf("the owner's password stopped working after a refused takeover: %d", rec.Code)
	}
}

func TestPasswordSignIn(t *testing.T) {
	_, h := newSignedInServer(t)
	if rec := postJSON(h, AuthSetupPath, ownerCreds, nil); rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body)
	}

	for _, body := range []string{
		`{"login":"liam","password":"wrong password"}`,
		`{"login":"someone","password":"correct horse"}`,
		`{"login":"","password":""}`,
	} {
		rec := postJSON(h, AuthLoginPath, body, nil)
		if rec.Code != http.StatusUnauthorized || sessionCookie(rec) != nil {
			t.Errorf("login %s = %d (cookie %v), want 401 and no session", body, rec.Code, sessionCookie(rec) != nil)
		}
	}

	// Usernames are case-insensitive and trimmed, as they were stored.
	rec := postJSON(h, AuthLoginPath, `{"login":"  LIAM ","password":"correct horse"}`, nil)
	cookie := sessionCookie(rec)
	if rec.Code != http.StatusOK || cookie == nil {
		t.Fatalf("login = %d, cookie %v", rec.Code, cookie)
	}
	if rec := getWithCookie(h, "/api/v1/bootstrap", cookie); rec.Code != http.StatusOK {
		t.Errorf("signed in by password: GET /bootstrap = %d", rec.Code)
	}
}

// Before a password exists there's nothing to log in with, even the seeded
// random hash: the owner hasn't chosen one.
func TestLoginRefusedBeforeSetup(t *testing.T) {
	_, h := newSignedInServer(t)
	if rec := postJSON(h, AuthLoginPath, ownerCreds, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("login before setup = %d, want 401", rec.Code)
	}
	if got := readStatus(t, h, nil); got.Claimed {
		t.Error("a failed login claimed the instance")
	}
}

// An install claimed before passwords existed stays closed to strangers, but
// its signed-in browser can add a password for other devices.
func TestClaimedInstallSetsPasswordOnlyWhenSignedIn(t *testing.T) {
	pool := testdb.New(t)
	if _, err := seed.Run(context.Background(), pool, seed.Options{UserName: "Liam"}); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	h := (&Server{Pool: pool, Q: q, Tracker: &tracker.Service{Pool: pool}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	cookie := signIn(t, q, h)
	// What migration 00102 does for an install that already had a session.
	if _, err := pool.Exec(context.Background(), `UPDATE users SET claimed_at = now()`); err != nil {
		t.Fatal(err)
	}

	if rec := postJSON(h, AuthSetupPath, ownerCreds, nil); !wantsBrowserSession(rec) {
		t.Errorf("setup on a claimed install, signed out = %d %s, want 401", rec.Code, rec.Body)
	}
	rec := postJSON(h, AuthSetupPath, ownerCreds, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup while signed in = %d %s", rec.Code, rec.Body)
	}
	if sessionCookie(rec) != nil {
		t.Error("a signed-in browser was given a second session")
	}
	if rec := postJSON(h, AuthLoginPath, ownerCreds, nil); rec.Code != http.StatusOK {
		t.Errorf("login with the new password = %d", rec.Code)
	}
}

func TestSetupValidatesAndRefusesCrossOrigin(t *testing.T) {
	_, h := newSignedInServer(t)
	for body, field := range map[string]string{
		`{"login":"liam","password":"short"}`:            "password",
		`{"login":"has space","password":"long enough"}`: "login",
		`{"login":"","password":"long enough"}`:          "login",
	} {
		rec := postJSON(h, AuthSetupPath, body, nil)
		var e struct{ Field string }
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != http.StatusBadRequest || e.Field != field {
			t.Errorf("setup %s = %d field %q, want 400 on %q", body, rec.Code, e.Field, field)
		}
	}

	// A playground runs agent-written code on this host and the browser treats
	// it as same-site: it mustn't be able to claim a fresh install.
	if rec := postJSON(h, AuthSetupPath, ownerCreds, nil, "Sec-Fetch-Site", "same-site"); rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin setup = %d, want 403", rec.Code)
	}
	if got := readStatus(t, h, nil); got.Claimed {
		t.Error("a refused setup claimed the instance")
	}
}

func TestLoginThrottlesGuesses(t *testing.T) {
	_, h := newSignedInServer(t)
	if rec := postJSON(h, AuthSetupPath, ownerCreds, nil); rec.Code != http.StatusOK {
		t.Fatalf("setup = %d", rec.Code)
	}
	for range maxLoginFailures {
		if rec := postJSON(h, AuthLoginPath, `{"login":"liam","password":"guess"}`, nil); rec.Code != http.StatusUnauthorized {
			t.Fatalf("guess = %d, want 401", rec.Code)
		}
	}
	rec := postJSON(h, AuthLoginPath, ownerCreds, nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || sessionCookie(rec) != nil {
		t.Errorf("after %d failures, the right password = %d (Retry-After %q), want 429 and no session", maxLoginFailures, rec.Code, rec.Header().Get("Retry-After"))
	}
}

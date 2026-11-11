package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/auth"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Password sign-in for the instance's owner. A fresh install is unclaimed:
// the first browser to open it picks a username and password at
// AuthSetupPath, which claims it for good. After that, AuthLoginPath trades
// them for a session, and `make signin` stays the recovery path. These
// routes answer browsers that aren't signed in yet, so the session check
// skips them (personFacing) and each enforces what it needs itself.
const (
	AuthStatusPath = "/api/v1/auth/status"
	AuthSetupPath  = "/api/v1/auth/setup"
	AuthLoginPath  = "/api/v1/auth/login"
)

const (
	minPasswordLen = 8
	// argon2id hashes whatever it's given; bound the work an anonymous caller can ask for.
	maxPasswordLen = 256
	maxAuthBody    = 4 << 10
)

var loginPattern = regexp.MustCompile(`^[a-z0-9._@-]{1,64}$`)

type authStatus struct {
	Claimed     bool `json:"claimed"`
	PasswordSet bool `json:"password_set"`
	SignedIn    bool `json:"signed_in"`
}

type authCredentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusServiceUnavailable, api.NoUser, "no workspace user yet", "")
		return
	}
	signedIn, err := s.hasSession(r)
	if err != nil {
		s.authFailed(w, "auth status", err)
		return
	}
	writeAuthStatus(w, u, signedIn)
}

// authSetup sets the owner's username and password: on an unclaimed install
// for anyone (first run), on a claimed one without a password only for a
// signed-in browser. A browser that wasn't signed in comes away with a session.
func (s *Server) authSetup(w http.ResponseWriter, r *http.Request) {
	if crossOriginWrite(r) {
		writeError(w, http.StatusForbidden, api.Forbidden, "cross-origin writes are refused", "")
		return
	}
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusServiceUnavailable, api.NoUser, "no workspace user yet", "")
		return
	}
	creds, field, err := readCredentials(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, api.Invalid, err.Error(), field)
		return
	}
	signedIn, err := s.hasSession(r)
	if err != nil {
		s.authFailed(w, "auth setup", err)
		return
	}
	hash, err := auth.HashPassword(creds.Password)
	if err != nil {
		s.authFailed(w, "hash password", err)
		return
	}
	updated, err := s.Q.SetOwnerPassword(r.Context(), store.SetOwnerPasswordParams{ID: u.ID, Login: &creds.Login, PasswordHash: hash, SignedIn: signedIn})
	switch {
	case errors.Is(err, pgx.ErrNoRows) && u.PasswordSetAt != nil:
		writeError(w, http.StatusConflict, api.Conflict, "Vesper is already set up: sign in with your username and password", "")
		return
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusUnauthorized, api.Unauthenticated, signInMessage, "")
		return
	case err != nil:
		s.authFailed(w, "set owner password", err)
		return
	}
	if !signedIn {
		if !s.startSession(w, r) {
			return
		}
	}
	writeAuthStatus(w, updated, true)
}

// authLogin trades the owner's username and password for a session.
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if crossOriginWrite(r) {
		writeError(w, http.StatusForbidden, api.Forbidden, "cross-origin writes are refused", "")
		return
	}
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusServiceUnavailable, api.NoUser, "no workspace user yet", "")
		return
	}
	var creds authCredentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthBody)).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, api.Invalid, "send a JSON body with login and password", "")
		return
	}
	if wait := s.logins.wait(time.Now()); wait > 0 {
		w.Header().Set("Retry-After", fmt.Sprint(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, api.Unauthenticated, "Too many failed sign-ins. Wait a few minutes, or use `make signin`.", "")
		return
	}
	ok, err := s.logins.check(u, normalizeLogin(creds.Login), creds.Password)
	if err != nil {
		s.authFailed(w, "check password", err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, api.Unauthenticated, "That username and password don't match.", "")
		return
	}
	if s.startSession(w, r) {
		writeAuthStatus(w, u, true)
	}
}

// hasSession reports whether r comes from a signed-in browser.
func (s *Server) hasSession(r *http.Request) (bool, error) {
	if s.AnonymousAPI {
		return true, nil
	}
	var token string
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		token = c.Value
	}
	return auth.ValidSession(r.Context(), s.Q, token)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) bool {
	token, err := auth.NewSession(r.Context(), s.Q)
	if err != nil {
		s.authFailed(w, "start session", err)
		return false
	}
	setSessionCookie(w, r, token)
	return true
}

func (s *Server) authFailed(w http.ResponseWriter, what string, err error) {
	s.Log.Error(what, "err", err)
	writeError(w, http.StatusInternalServerError, api.Internal, "internal error", "")
}

func writeAuthStatus(w http.ResponseWriter, u store.User, signedIn bool) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(authStatus{Claimed: u.ClaimedAt != nil, PasswordSet: u.PasswordSetAt != nil, SignedIn: signedIn})
}

func normalizeLogin(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// readCredentials decodes and validates a new username and password,
// returning the offending field with the error.
func readCredentials(w http.ResponseWriter, r *http.Request) (authCredentials, string, error) {
	var c authCredentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthBody)).Decode(&c); err != nil {
		return c, "", errors.New("send a JSON body with login and password")
	}
	c.Login = normalizeLogin(c.Login)
	if !loginPattern.MatchString(c.Login) {
		return c, "login", errors.New("use 1 to 64 letters, digits, or . _ @ -")
	}
	if n := utf8.RuneCountInString(c.Password); n < minPasswordLen || n > maxPasswordLen {
		return c, "password", fmt.Errorf("use %d to %d characters", minPasswordLen, maxPasswordLen)
	}
	return c, "", nil
}

// loginLimiter throttles password guesses. Requests arrive through the web
// proxy, so the client address isn't the browser's: the limit is global,
// which suits one owner, and `make signin` still works while it's tripped.
// Checks run one at a time so parallel guesses can't multiply argon2's
// memory or slip past the count.
type loginLimiter struct {
	mu       sync.Mutex
	failures []time.Time
}

const (
	loginFailureWindow = 15 * time.Minute
	maxLoginFailures   = 10
)

// wait is how long until another attempt is allowed; 0 means now.
func (l *loginLimiter) wait(now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	if len(l.failures) < maxLoginFailures {
		return 0
	}
	return l.failures[0].Add(loginFailureWindow).Sub(now)
}

func (l *loginLimiter) prune(now time.Time) {
	i := 0
	for i < len(l.failures) && now.Sub(l.failures[i]) >= loginFailureWindow {
		i++
	}
	l.failures = l.failures[i:]
}

// check verifies the owner's credentials and records a failure. It always
// hashes, so a wrong username takes as long as a wrong password.
func (l *loginLimiter) check(u store.User, login, password string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	passwordOK, err := auth.CheckPassword(u.PasswordHash, password)
	if err != nil {
		return false, err
	}
	loginOK := u.Login != nil && subtle.ConstantTimeCompare([]byte(*u.Login), []byte(login)) == 1
	if ok := u.PasswordSetAt != nil && loginOK && passwordOK; ok {
		l.failures = nil
		return true, nil
	}
	l.failures = append(l.failures, time.Now())
	return false, nil
}

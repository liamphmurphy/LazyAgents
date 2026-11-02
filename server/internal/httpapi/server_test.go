package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

func TestHealth(t *testing.T) {
	ok := pingFunc(func(context.Context) error { return nil })
	bad := pingFunc(func(context.Context) error { return errors.New("connection refused") })

	tests := []struct {
		name       string
		checks     map[string]Pinger
		wantCode   int
		wantStatus string
	}{
		{"all ok", map[string]Pinger{"postgres": ok, "s3": ok}, http.StatusOK, "ok"},
		{"one down", map[string]Pinger{"postgres": ok, "s3": bad}, http.StatusServiceUnavailable, "degraded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &Server{Version: "test", Checks: tt.checks, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			h := srv.Handler()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			var body struct {
				Status string `json:"status"`
				Checks map[string]struct {
					OK    bool   `json:"ok"`
					Error string `json:"error"`
				} `json:"checks"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", body.Status, tt.wantStatus)
			}
			if len(body.Checks) != len(tt.checks) {
				t.Errorf("checks = %d, want %d", len(body.Checks), len(tt.checks))
			}
			if s3, found := body.Checks["s3"]; found && !s3.OK && s3.Error == "" {
				t.Error("failed check should carry its error")
			}
		})
	}
}

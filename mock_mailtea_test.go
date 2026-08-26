package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A tiny stand-in for the Mailtea API, so these tests run with no credentials
// and no network. It records every request it receives, which is what the
// assertions read.

const mockEmailID = "txemail_00000000000000000000000000000000"

type recordedRequest struct {
	Method        string
	Path          string
	Authorization string
	Body          map[string]interface{}
}

type mockMailtea struct {
	*httptest.Server

	mu       sync.Mutex
	requests []recordedRequest
}

// startMockMailtea boots the mock and shuts it down when the test ends.
func startMockMailtea(t *testing.T) *mockMailtea {
	t.Helper()

	mock := &mockMailtea{}
	mock.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}

		authorization := r.Header.Get("Authorization")
		mock.mu.Lock()
		mock.requests = append(mock.requests, recordedRequest{
			Method:        r.Method,
			Path:          r.URL.Path,
			Authorization: authorization,
			Body:          body,
		})
		mock.mu.Unlock()

		send := func(status int, payload map[string]interface{}) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(payload)
		}

		// Auth is checked first, the same way the real API does it — an example
		// that forgets the key should fail its test, not silently "send".
		if !strings.HasPrefix(authorization, "Bearer ") || strings.TrimPrefix(authorization, "Bearer ") == "" {
			send(http.StatusUnauthorized, map[string]interface{}{"error": "Unauthorized"})
			return
		}

		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/emails/"), "/cancel")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/emails":
			send(http.StatusOK, map[string]interface{}{"id": mockEmailID})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/emails/"):
			send(http.StatusOK, map[string]interface{}{
				"object":     "email",
				"id":         id,
				"subject":    "Mock email",
				"last_event": "scheduled",
				"created_at": "2026-01-01T00:00:00.000Z",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
			send(http.StatusOK, map[string]interface{}{"object": "email", "id": id})
		default:
			send(http.StatusNotFound, map[string]interface{}{"error": "Not Found", "path": r.URL.Path})
		}
	}))
	t.Cleanup(mock.Close)

	return mock
}

func (m *mockMailtea) all() []recordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]recordedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}

// last is the most recent request, which is what most assertions want.
func (m *mockMailtea) last(t *testing.T) recordedRequest {
	t.Helper()
	all := m.all()
	if len(all) == 0 {
		t.Fatal("the mock received no requests")
	}
	return all[len(all)-1]
}

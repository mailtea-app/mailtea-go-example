package mailtea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The tests in the parent package always hand the client a BaseURL, because
// they point it at a mock. That leaves the branch every real caller takes —
// New with no BaseURL at all — as the one line nothing exercises, and a typo
// there sends production mail nowhere while the whole suite stays green.
func TestNewDefaultsToTheHostedAPI(t *testing.T) {
	for name, given := range map[string]string{
		"unset":            "",
		"whitespace":       "   ",
		"trailing slash":   DefaultBaseURL + "/",
		"trailing slashes": DefaultBaseURL + "///",
	} {
		t.Run(name, func(t *testing.T) {
			if got := New("mt_pat_test_key", Options{BaseURL: given}).baseURL; got != DefaultBaseURL {
				t.Errorf("baseURL = %q, want %q", got, DefaultBaseURL)
			}
		})
	}

	if DefaultBaseURL != "https://api.mailtea.app" {
		t.Errorf("DefaultBaseURL = %q, want https://api.mailtea.app", DefaultBaseURL)
	}

	// A self-hosted URL keeps its path prefix; only the trailing slash goes,
	// so paths concatenate to one slash rather than two.
	if got := New("k", Options{BaseURL: "https://mail.example.internal/api/"}).baseURL; got != "https://mail.example.internal/api" {
		t.Errorf("baseURL = %q, want the trailing slash trimmed and the path kept", got)
	}
}

// A gateway or proxy in front of Mailtea answers with HTML, not JSON. The
// example prints Message on failure, so it has to say something either way.
func TestAPIErrorMessageSurvivesANonJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		// Deliberately empty: the emptiest possible body still has to name the
		// status, or the operator is told only that "something" failed.
	}))
	defer server.Close()

	client := New("mt_pat_test_key", Options{BaseURL: server.URL})
	_, err := client.SendEmail(context.Background(), SendEmailRequest{
		From:    "Acme <hello@acme.com>",
		To:      []string{"reader@yourdomain.com"},
		Subject: "Gateway",
		Text:    "Gateway",
	})

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %v, want a *APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", apiErr.StatusCode)
	}
	if apiErr.Message != http.StatusText(http.StatusBadGateway) {
		t.Errorf("message = %q, want %q", apiErr.Message, http.StatusText(http.StatusBadGateway))
	}
}

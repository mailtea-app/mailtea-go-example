package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mailtea-app/mailtea-go-example/mailtea"
)

func TestSendEmailPostsToTheAPI(t *testing.T) {
	mock := startMockMailtea(t)
	client := mailtea.New("mt_pat_test_key", mailtea.Options{BaseURL: mock.URL})

	sent, err := client.SendEmail(context.Background(), mailtea.SendEmailRequest{
		From:    "Acme <hello@acme.com>",
		To:      []string{"reader@yourdomain.com"},
		Subject: "Hello from Go",
		HTML:    "<p>Hi</p>",
		Tags:    []mailtea.Tag{{Name: "example", Value: "go"}},
	})
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	req := mock.last(t)
	if req.Method != http.MethodPost || req.Path != "/v1/emails" {
		t.Errorf("got %s %s, want POST /v1/emails", req.Method, req.Path)
	}
	if req.Authorization != "Bearer mt_pat_test_key" {
		t.Errorf("Authorization = %q, want %q", req.Authorization, "Bearer mt_pat_test_key")
	}
	assertField(t, req.Body, "from", "Acme <hello@acme.com>")
	assertField(t, req.Body, "subject", "Hello from Go")
	assertField(t, req.Body, "html", "<p>Hi</p>")
	assertRecipients(t, req.Body, "reader@yourdomain.com")

	// The id is the whole point of the call: it is what you look the send up
	// by and what webhooks reference.
	if sent.ID != mockEmailID {
		t.Errorf("id = %q, want %q", sent.ID, mockEmailID)
	}
}

func TestSendEmailSchedulesWithScheduledAt(t *testing.T) {
	mock := startMockMailtea(t)
	client := mailtea.New("mt_pat_test_key", mailtea.Options{BaseURL: mock.URL})

	if _, err := client.SendEmail(context.Background(), mailtea.SendEmailRequest{
		From:        "Acme <hello@acme.com>",
		To:          []string{"reader@yourdomain.com"},
		Subject:     "Later",
		Text:        "Later",
		ScheduledAt: "2026-09-01T09:00:00Z",
	}); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	assertField(t, mock.last(t).Body, "scheduled_at", "2026-09-01T09:00:00Z")
}

func TestGetAndCancelHitTheRightRoutes(t *testing.T) {
	mock := startMockMailtea(t)
	client := mailtea.New("mt_pat_test_key", mailtea.Options{BaseURL: mock.URL})

	email, err := client.GetEmail(context.Background(), mockEmailID)
	if err != nil {
		t.Fatalf("GetEmail: %v", err)
	}
	if got := mock.last(t); got.Method != http.MethodGet || got.Path != "/v1/emails/"+mockEmailID {
		t.Errorf("got %s %s, want GET /v1/emails/%s", got.Method, got.Path, mockEmailID)
	}
	if email.LastEvent != "scheduled" {
		t.Errorf("last_event = %q, want %q", email.LastEvent, "scheduled")
	}

	if _, err := client.CancelEmail(context.Background(), mockEmailID); err != nil {
		t.Fatalf("CancelEmail: %v", err)
	}
	if got := mock.last(t); got.Method != http.MethodPost || got.Path != "/v1/emails/"+mockEmailID+"/cancel" {
		t.Errorf("got %s %s, want POST /v1/emails/%s/cancel", got.Method, got.Path, mockEmailID)
	}
}

func TestMissingKeyIsRejected(t *testing.T) {
	mock := startMockMailtea(t)
	client := mailtea.New("", mailtea.Options{BaseURL: mock.URL})

	_, err := client.SendEmail(context.Background(), mailtea.SendEmailRequest{
		From:    "Acme <hello@acme.com>",
		To:      []string{"reader@yourdomain.com"},
		Subject: "No key",
		Text:    "No key",
	})

	var apiErr *mailtea.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *mailtea.APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", apiErr.StatusCode)
	}
}

func TestAPIErrorSurfacesTheAPIMessage(t *testing.T) {
	// This is the case people copy into production: the API rejected the send
	// and said why, so the message has to survive the trip back to the caller.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"Domain is not verified","details":[{"path":["from"]}]}`))
	}))
	defer server.Close()

	client := mailtea.New("mt_pat_test_key", mailtea.Options{BaseURL: server.URL})
	_, err := client.SendEmail(context.Background(), mailtea.SendEmailRequest{
		From:    "Acme <hello@unverified.example.org>",
		To:      []string{"reader@yourdomain.com"},
		Subject: "Nope",
		Text:    "Nope",
	})

	var apiErr *mailtea.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *mailtea.APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", apiErr.StatusCode)
	}
	if apiErr.Message != "Domain is not verified" {
		t.Errorf("message = %q, want %q", apiErr.Message, "Domain is not verified")
	}
	if !strings.Contains(err.Error(), "Domain is not verified") {
		t.Errorf("Error() = %q, want it to carry the API message", err.Error())
	}
	if !strings.Contains(apiErr.Body, `"details"`) {
		t.Errorf("Body = %q, want the raw response including details", apiErr.Body)
	}
}

func TestRunWalksTheWholeFlow(t *testing.T) {
	mock := startMockMailtea(t)
	client := mailtea.New("mt_pat_test_key", mailtea.Options{BaseURL: mock.URL})

	var out bytes.Buffer
	d := demo{from: "Acme <hello@acme.com>", to: "reader@yourdomain.com", subject: "Hello from Go (test)"}
	if err := run(context.Background(), client, d, &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	want := []string{
		"POST /v1/emails",
		"POST /v1/emails",
		"GET /v1/emails/" + mockEmailID,
		"POST /v1/emails/" + mockEmailID + "/cancel",
	}
	requests := mock.all()
	if len(requests) != len(want) {
		t.Fatalf("got %d requests, want %d: %+v", len(requests), len(want), requests)
	}
	for i, req := range requests {
		if got := req.Method + " " + req.Path; got != want[i] {
			t.Errorf("request %d = %q, want %q", i, got, want[i])
		}
		if req.Authorization != "Bearer mt_pat_test_key" {
			t.Errorf("request %d Authorization = %q", i, req.Authorization)
		}
	}

	assertField(t, requests[0].Body, "subject", "Hello from Go (test)")
	assertField(t, requests[1].Body, "subject", "Hello from Go (test) (scheduled)")
	if requests[1].Body["scheduled_at"] == nil {
		t.Error("the second send carried no scheduled_at")
	}

	// The example has to show the operator the id it got back.
	if !strings.Contains(out.String(), mockEmailID) {
		t.Errorf("output does not mention the email id:\n%s", out.String())
	}
}

func TestLoadEnvFileDoesNotOverrideRealEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	if err := os.WriteFile(path, []byte("# comment\nMAILTEA_FROM=\"Acme <hello@acme.com>\"\nMAILTEA_TO=reader@yourdomain.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MAILTEA_TO", "already-set@yourdomain.com")
	t.Setenv("MAILTEA_FROM", "")
	os.Unsetenv("MAILTEA_FROM")

	if err := loadEnvFile(path); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if got := os.Getenv("MAILTEA_FROM"); got != "Acme <hello@acme.com>" {
		t.Errorf("MAILTEA_FROM = %q, want the value from the file with quotes stripped", got)
	}
	if got := os.Getenv("MAILTEA_TO"); got != "already-set@yourdomain.com" {
		t.Errorf("MAILTEA_TO = %q, want the real environment to win", got)
	}
}

func assertField(t *testing.T, body map[string]interface{}, key, want string) {
	t.Helper()
	got, _ := body[key].(string)
	if got != want {
		t.Errorf("body[%q] = %q, want %q", key, got, want)
	}
}

func assertRecipients(t *testing.T, body map[string]interface{}, want ...string) {
	t.Helper()
	raw, _ := json.Marshal(body["to"])
	var got []string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("body[\"to\"] = %s, want an array of addresses", raw)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("body[\"to\"] = %v, want %v", got, want)
	}
}

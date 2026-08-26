// Command mailtea-go-example walks the transactional email path against the
// Mailtea HTTP API: send one now, schedule one for later, look it up, cancel it.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mailtea-app/mailtea-go-example/mailtea"
)

func main() {
	log.SetFlags(0)

	if err := loadEnvFile(".env"); err != nil {
		log.Fatalf("reading .env: %v", err)
	}

	apiKey := os.Getenv("MAILTEA_API_KEY")
	if apiKey == "" {
		log.Fatal("MAILTEA_API_KEY is not set. Copy .env.example to .env and add your key.")
	}

	client := mailtea.New(apiKey, mailtea.Options{
		// Only needed for local dev or a self-hosted Mailtea. Unset in
		// production, and the client falls back to https://api.mailtea.app.
		BaseURL: os.Getenv("MAILTEA_API_BASE_URL"),
	})

	d := demo{
		from: envOr("MAILTEA_FROM", "Acme <hello@acme.com>"),
		to:   envOr("MAILTEA_TO", "reader@yourdomain.com"),
		// A per-run suffix, so repeat runs are easy to tell apart in the
		// Mailtea dashboard and in the recipient's inbox.
		subject: "Hello from Go (" + runSuffix() + ")",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := run(ctx, client, d, os.Stdout); err != nil {
		// The API says why in plain words — a bad key, an unverified domain, a
		// field it rejected. Printing only "request failed" throws that away.
		var apiErr *mailtea.APIError
		if errors.As(err, &apiErr) {
			log.Fatalf("%v\nresponse: %s", err, apiErr.Body)
		}
		log.Fatal(err)
	}
}

type demo struct {
	from    string
	to      string
	subject string
}

// run drives whichever Mailtea the client points at. main wires it to the real
// API; the tests point it at a mock.
func run(ctx context.Context, client *mailtea.Client, d demo, out io.Writer) error {
	// 1. Send one email now.
	sent, err := client.SendEmail(ctx, mailtea.SendEmailRequest{
		From:    d.from,
		To:      []string{d.to},
		Subject: d.subject,
		HTML:    "<p>Sent from a Go program, straight through the Mailtea HTTP API.</p>",
		Text:    "Sent from a Go program, straight through the Mailtea HTTP API.",
		Tags:    []mailtea.Tag{{Name: "example", Value: "go"}},
	})
	if err != nil {
		return fmt.Errorf("send failed: %w", err)
	}
	fmt.Fprintf(out, "sent:      %s  %s\n", sent.ID, d.subject)

	// 2. Schedule one. Same call, plus scheduled_at.
	scheduledAt := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	scheduled, err := client.SendEmail(ctx, mailtea.SendEmailRequest{
		From:        d.from,
		To:          []string{d.to},
		Subject:     d.subject + " (scheduled)",
		HTML:        "<p>Queued an hour ahead with scheduled_at.</p>",
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		return fmt.Errorf("scheduled send failed: %w", err)
	}
	fmt.Fprintf(out, "scheduled: %s  for %s\n", scheduled.ID, scheduledAt)

	// 3. Look it up.
	email, err := client.GetEmail(ctx, scheduled.ID)
	if err != nil {
		return fmt.Errorf("lookup failed: %w", err)
	}
	fmt.Fprintf(out, "lookup:    %s  last_event=%s\n", email.ID, email.LastEvent)

	// 4. Cancel it while it is still queued.
	canceled, err := client.CancelEmail(ctx, scheduled.ID)
	if err != nil {
		return fmt.Errorf("cancel failed: %w", err)
	}
	fmt.Fprintf(out, "canceled:  %s\n", canceled.ID)

	return nil
}

func runSuffix() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("150405")
	}
	return hex.EncodeToString(buf)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// loadEnvFile reads a .env file into the process environment. Go ships no
// dotenv in its standard library and this example takes no dependencies, so
// these are the twenty lines that make `cp .env.example .env` work. Real
// environment variables win, and a missing file is not an error.
func loadEnvFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, alreadySet := os.LookupEnv(key); !alreadySet {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

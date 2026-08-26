# Mailtea + Go Example

This example shows how to use [Mailtea](https://mailtea.app) with Go to send,
schedule, look up, and cancel transactional email.

There is no official Mailtea SDK for Go, so this calls the HTTP API directly
with `net/http` and `encoding/json` — the standard library and nothing else. The
client is one file, [`mailtea/client.go`](mailtea/client.go), written to be read
in one sitting and copied into your own project.

## Prerequisites

To get the most out of this guide, you'll need to:

- [Create an API key](https://studio.mailtea.app/api-keys)
- [Verify your domain](https://docs.mailtea.app/docs/documentation/domains)

Go 1.18 or newer. There is nothing to download.

## Instructions

1. Install dependencies — there are none, so this just checks the module builds:
   ```bash
   go build ./...
   ```
2. Copy `.env.example` to `.env` and add your API key:
   ```bash
   cp .env.example .env
   ```
3. Run it:
   ```bash
   go run .
   ```

```
sent:      txemail_8d68ce5aa35949229fa4ab62b912ea02  Hello from Go (c710deec)
scheduled: txemail_682de31c8a034aa5ba0de55de198b81d  for 2026-08-26T15:22:36Z
lookup:    txemail_682de31c8a034aa5ba0de55de198b81d  last_event=scheduled
canceled:  txemail_682de31c8a034aa5ba0de55de198b81d
```

Go has no dotenv in its standard library, and this example takes no
dependencies, so `main.go` reads `.env` itself in about twenty lines. Real
environment variables win over the file, and a missing file is not an error —
export `MAILTEA_API_KEY` instead if you would rather.

## Using the client

```go
client := mailtea.New(os.Getenv("MAILTEA_API_KEY"), mailtea.Options{
	// Only needed for local dev or a self-hosted Mailtea. Omit in production
	// and the client uses https://api.mailtea.app.
	BaseURL: os.Getenv("MAILTEA_API_BASE_URL"),
})
```

### Send an email

```go
sent, err := client.SendEmail(ctx, mailtea.SendEmailRequest{
	From:    "Acme <hello@acme.com>",
	To:      []string{"reader@yourdomain.com"},
	Subject: "Hello from Go",
	HTML:    "<p>Sent with the Mailtea API.</p>",
	Text:    "Sent with the Mailtea API.",
	Tags:    []mailtea.Tag{{Name: "example", Value: "go"}},
})
// sent.ID == "txemail_8d68ce5aa35949229fa4ab62b912ea02"
```

`To`, `CC`, `BCC`, and `ReplyTo` are slices. A single message is capped at **50
recipients combined** across `to` + `cc` + `bcc`.

### Schedule one, then cancel it

Scheduling is the same call with `ScheduledAt` set to an RFC 3339 timestamp.

```go
scheduled, err := client.SendEmail(ctx, mailtea.SendEmailRequest{
	From:        "Acme <hello@acme.com>",
	To:          []string{"reader@yourdomain.com"},
	Subject:     "Hello from Go (scheduled)",
	HTML:        "<p>Queued an hour ahead.</p>",
	ScheduledAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
})

_, err = client.CancelEmail(ctx, scheduled.ID)
```

Cancelling works only while the email is still `scheduled`. Any other status —
including the `queued` of an ordinary immediate send — answers 422, so cancel is
for scheduled mail only, not an undo button on a send already on its way.

### Look up a send

```go
email, err := client.GetEmail(ctx, sent.ID)
// queued, scheduled, sent, delivered, delivery_delayed,
// bounced, complained, failed, suppressed, canceled
email.LastEvent
```

`LastEvent` is how you check on a send without setting up a webhook.

### Handling errors

Every non-2xx comes back as a `*mailtea.APIError` carrying the status, the
API's own message, and the raw body — which holds the `details` array naming
the field that failed validation.

```go
var apiErr *mailtea.APIError
if errors.As(err, &apiErr) {
	log.Fatalf("%v\nresponse: %s", err, apiErr.Body)
}
```

```
send failed: mailtea: API error 401: Unauthorized
response: {"error":"Unauthorized"}
```

Surfacing that message is the difference between "the send failed" and "the
domain isn't verified yet".

### Endpoints

| Client method | Request |
|---|---|
| `SendEmail` | `POST /v1/emails` |
| `SendEmail` with `ScheduledAt` | `POST /v1/emails` |
| `GetEmail` | `GET /v1/emails/{id}` |
| `CancelEmail` | `POST /v1/emails/{id}/cancel` |

## What this example covers

- Calling the Mailtea HTTP API from Go with no SDK and no dependencies
- Sending an email with `html`, `text`, and `tags`
- Scheduling a send with `scheduled_at`, then cancelling it
- Reading a send's `last_event` to see where it got to
- Typed errors: `*mailtea.APIError` surfaces the API's own message on any non-2xx
- Keeping the API key in the environment, never in the source or in git

## Tests

```bash
go test ./...
```

The tests run against a bundled mock Mailtea server, so they need no API key
and make no network calls. The mock is a stdlib `httptest.Server` in
[`mock_mailtea_test.go`](mock_mailtea_test.go): it records every request it
receives and rejects the ones with no bearer token, so the assertions can check
the method, path, `Authorization` header, and body of each one.

## Learn more

- [Documentation](https://docs.mailtea.app)
- [API reference](https://docs.mailtea.app/docs/api-reference)
- [Node.js SDK](https://github.com/mailtea-app/mailtea-node) ·
  [Python SDK](https://github.com/mailtea-app/mailtea-python) ·
  [MCP server](https://github.com/mailtea-app/mailtea-mcp)

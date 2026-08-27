# Mailtea + Go Example

This example shows how to use [Mailtea](https://mailtea.app) with Go to send,
schedule, look up, and cancel transactional email.

It uses the official Go SDK,
[`github.com/mailtea-app/mailtea-go`](https://github.com/mailtea-app/mailtea-go) —
a thin, typed wrapper over the REST API with no dependencies outside the
standard library.

## Prerequisites

To get the most out of this guide, you'll need to:

- [Create an API key](https://studio.mailtea.app/api-keys)
- [Verify your domain](https://docs.mailtea.app/docs/documentation/domains)

Go 1.18 or newer.

## Instructions

1. Fetch the SDK:
   ```bash
   go mod tidy
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
lookup:    txemail_682de31c8a034aa5ba0de55de198b81d  status=scheduled
canceled:  txemail_682de31c8a034aa5ba0de55de198b81d
```

Go has no dotenv in its standard library, and this example takes no
dependencies beyond the SDK, so `main.go` reads `.env` itself in about twenty
lines. Real environment variables win over the file, and a missing file is not
an error — export `MAILTEA_API_KEY` instead if you would rather.

## Using the SDK

```go
import "github.com/mailtea-app/mailtea-go" // package mailtea

// The key comes from MAILTEA_API_KEY when you pass "", and the base URL from
// MAILTEA_API_BASE_URL — only needed for local dev or a self-hosted Mailtea.
// Unset in production and the SDK uses https://api.mailtea.app.
client, err := mailtea.New(os.Getenv("MAILTEA_API_KEY"))
```

### Send an email

```go
sent, err := client.Emails.Send(ctx, mailtea.SendEmailRequest{
	From:    "Acme <hello@acme.com>",
	To:      []string{"reader@yourdomain.com"},
	Subject: "Hello from Go",
	HTML:    "<p>Sent with the Mailtea SDK.</p>",
	Text:    "Sent with the Mailtea SDK.",
	Tags:    []mailtea.Tag{{Name: "example", Value: "go"}},
})
// sent.ID == "txemail_8d68ce5aa35949229fa4ab62b912ea02"
```

`To`, `CC`, `BCC`, and `ReplyTo` are slices. A single message is capped at **50
recipients combined** across `to` + `cc` + `bcc`.

### Schedule one, then cancel it

Scheduling is the same call with `ScheduledAt` set to an RFC 3339 timestamp.

```go
scheduled, err := client.Emails.Send(ctx, mailtea.SendEmailRequest{
	From:        "Acme <hello@acme.com>",
	To:          []string{"reader@yourdomain.com"},
	Subject:     "Hello from Go (scheduled)",
	HTML:        "<p>Queued an hour ahead.</p>",
	ScheduledAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
})

_, err = client.Emails.Cancel(ctx, scheduled.ID)
```

Cancelling works only while the email is still `scheduled`. Any other status —
including the `queued` of an ordinary immediate send — answers 422, so cancel is
for scheduled mail only, not an undo button on a send already on its way.

### Look up a send

```go
email, err := client.Emails.Get(ctx, sent.ID)
// queued, scheduled, sent, delivered, delivery_delayed,
// bounced, complained, failed, suppressed, canceled
email.Status
```

`Status` is how you check on a send without setting up a webhook. It is the
SDK's friendly alias of the API's `last_event`, which `email.LastEvent` also
carries verbatim.

### Handling errors

Every failure comes back as a `*mailtea.Error` carrying the status, the API's
own message, its machine-readable `code`, the `x-request-id`, and the raw body —
which holds the `details` array naming the field that failed validation.

```go
var apiErr *mailtea.Error
if errors.As(err, &apiErr) {
	log.Fatalf("%v\nresponse: %s", err, apiErr.Body)
}
```

```
send failed: mailtea: Unauthorized (status 401, request id 0f0c…)
response: {"error":"Unauthorized"}
```

Surfacing that message is the difference between "the send failed" and "the
domain isn't verified yet".

### Endpoints

| SDK call | Request |
|---|---|
| `Emails.Send` | `POST /v1/emails` |
| `Emails.Send` with `ScheduledAt` | `POST /v1/emails` |
| `Emails.Get` | `GET /v1/emails/{id}` |
| `Emails.Cancel` | `POST /v1/emails/{id}/cancel` |

The SDK covers the rest of the API too — contacts, posts, topics, templates,
domains, webhooks, automations. See its
[README](https://github.com/mailtea-app/mailtea-go#api).

## What this example covers

- Calling Mailtea from Go with the official SDK
- Sending an email with `html`, `text`, and `tags`
- Scheduling a send with `ScheduledAt`, then cancelling it
- Reading a send's `Status` to see where it got to
- Typed errors: `*mailtea.Error` surfaces the API's own message on any non-2xx
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
- [Go SDK](https://github.com/mailtea-app/mailtea-go) ·
  [Node.js SDK](https://github.com/mailtea-app/mailtea-node) ·
  [Python SDK](https://github.com/mailtea-app/mailtea-python) ·
  [MCP server](https://github.com/mailtea-app/mailtea-mcp)

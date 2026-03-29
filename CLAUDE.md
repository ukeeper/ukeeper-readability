# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

ukeeper-readability is a Go web service that extracts article content from URLs using readability parsing. It has a MongoDB-backed rule system for per-domain custom CSS selectors, a JSON API, and an HTMX-driven web UI for managing extraction rules.

## Build and Development

```bash
go build -o ukeeper-readability .
go test -timeout=60s -race ./...              # all tests (datastore tests use testcontainers)
go test -timeout=60s -run TestName ./rest/     # single test
golangci-lint run --max-issues-per-linter=0 --max-same-issues=0  # lint from repo root
```

The `revision` variable in `main.go` is injected at build time: `-ldflags "-X main.revision=<version>"`.

Optional Cloudflare Browser Rendering flags (when both are set, uses `CloudflareRetriever` instead of default HTTP):
- `--cf-account-id` / `CF_ACCOUNT_ID` — Cloudflare account ID
- `--cf-api-token` / `CF_API_TOKEN` — Cloudflare API token with Browser Rendering Edit permission

`main_test.go` is gated behind `ENABLE_MONGO_TESTS=true` and needs MongoDB on localhost:27017. All other packages test independently — `datastore/` spins up MongoDB via testcontainers automatically.

## Architecture

```
main.go        → CLI flags (jessevdk/go-flags), wiring, startup
datastore/     → MongoDB access (RulesDAO, Rule struct)
extractor/     → URL fetching, content extraction, charset conversion
  mocks/       → moq-generated mock for Rules interface
rest/          → HTTP server, routing (go-pkgz/routegroup), handlers, basicAuth
web/           → Go HTML templates (HTMX v2), static assets
```

**Dependency flow:** `main → datastore, extractor, rest`; `rest → datastore, extractor`; `extractor → datastore` (Rule type + Rules interface).

**Key interfaces:**
- `extractor.Rules` (defined consumer-side in `extractor/readability.go`), implemented by `datastore.RulesDAO`. Mock generated with `//go:generate moq` in extractor package.
- `extractor.Retriever` (defined in `extractor/retriever.go`) — abstracts URL content fetching. Two implementations: `HTTPRetriever` (default, standard HTTP GET with Safari user-agent) and `CloudflareRetriever` (Cloudflare Browser Rendering API for JS-rendered pages). When `UReadability.Retriever` is nil, defaults to `HTTPRetriever`.

## Content Extraction Flow

1. Fetch URL via `Retriever` interface (default: HTTP GET with 30s timeout, Safari user-agent, follows redirects; optional: Cloudflare Browser Rendering for JS-heavy sites)
2. Detect charset from Content-Type header and `<meta>` tags, convert to UTF-8
3. Look up custom CSS selector rule from MongoDB by domain
4. If rule found → extract via goquery CSS selector; if fails → fall back to general parser
5. If no rule → use `go-readability` general parser
6. Normalize relative links to absolute, extract images concurrently (pick largest as lead image)

## Key Conventions

- Rule upsert is keyed on `domain` — one rule per domain. Rules are disabled (`enabled: false`), never deleted.
- `rest.Server.Readability` is `extractor.UReadability` by value (not pointer), with `Rules` interface field inside.
- Both `/api/extract` and `/api/content/v1/parser` require the `token` query parameter when configured. Token comparison uses `subtle.ConstantTimeCompare`. Protected rule management routes use custom `basicAuth` middleware with constant-time comparison.
- Web UI text is in Russian — tests assert on Russian strings, don't change them.
- Middleware stack: Recoverer → RealIP → AppInfo+Ping → Throttle(50) → Logger.
- CI: `ci.yml` runs tests and lint in the `build` job (MongoDB via service container); `docker.yml` builds Docker images via `workflow_run` trigger after `build` succeeds (no tests inside Docker).

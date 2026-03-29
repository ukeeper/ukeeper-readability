## ukeeper-readability [![build](https://github.com/ukeeper/ukeeper-readability/actions/workflows/ci.yml/badge.svg)](https://github.com/ukeeper/ukeeper-readability/actions/workflows/ci.yml) [![Coverage Status](https://coveralls.io/repos/github/ukeeper/ukeeper-readability/badge.svg?branch=master)](https://coveralls.io/github/ukeeper/ukeeper-readability?branch=master)

### Running instructions

`docker-compose up` will leave you with working ukeeper-readability service (both API and frontend) running on <http://localhost:8080>.

### Configuration

| Command line | Environment     | Default        | Description                                           |
|--------------|-----------------|----------------|-------------------------------------------------------|
| address      | UKEEPER_ADDRESS | all interfaces | web server listening address                          |
| port         | UKEEPER_PORT    | `8080`         | web server port                                       |
| mongo-uri    | MONGO_URI       | none           | MongoDB connection string, _required_                 |
| frontend-dir | FRONTEND_DIR    | `/srv/web`     | directory with frontend files                         |
| token        | UKEEPER_TOKEN   | none           | token for API endpoint auth                           |
| mongo-delay  | MONGO_DELAY     | `0`            | mongo initial delay                                   |
| mongo-db     | MONGO_DB        | `ureadability` | mongo database name                                   |
| creds        | CREDS           | none           | credentials for protected calls (POST, DELETE /rules) |
| cf-account-id| CF_ACCOUNT_ID   | none           | Cloudflare account ID for Browser Rendering API       |
| cf-api-token | CF_API_TOKEN    | none           | Cloudflare API token with Browser Rendering Edit perm |
| cf-route-all | CF_ROUTE_ALL    | `false`        | route every request through Cloudflare Browser Rendering |
| openai-api-key | OPENAI_API_KEY | none          | OpenAI API key; enables auto-evaluation when set      |
| openai-model | OPENAI_MODEL    | `gpt-5.4-mini` | OpenAI model for evaluation                           |
| openai-max-iter | OPENAI_MAX_ITER | `3`         | max evaluation iterations per extraction               |
| dbg          | DEBUG           | `false`        | debug mode                                            |

### Cloudflare Browser Rendering (optional)

Cloudflare Browser Rendering is useful for JavaScript-heavy pages and sites behind a "please enable JS" wall, but it's slower than direct HTTP and the free tier throttles at 1 request per 10 seconds. To keep the service cost-effective, Cloudflare routing is **opt-in**.

1. Set `--cf-account-id` and `--cf-api-token` to enable Cloudflare routing.
2. Either flip the `use_cloudflare` checkbox on individual rules (per-domain, recommended) or set `--cf-route-all=true` to route every request through Cloudflare.

When Cloudflare credentials are not set, the service uses a standard HTTP client for everything (default). On HTTP 429 (rate limit) the service automatically retries with exponential backoff and respects the `Retry-After` header.

### OpenAI Auto-Evaluation (optional)

When `--openai-api-key` is set, the service automatically evaluates extraction quality using OpenAI. If the extracted content looks poor (missing article body, too short, mostly boilerplate), GPT suggests a CSS selector targeting the main content. The service iterates up to `--openai-max-iter` times, saving the best selector as a rule for future use.

Evaluation only runs for domains without an existing extraction rule. For domains that already have rules, use the force-mode endpoint to re-evaluate:

    GET /api/content-parsed-wrong?url=http://example.com/article

This protected endpoint (requires basicAuth credentials) ignores the stored rule, re-extracts with the general parser, and runs the evaluation loop to find a better selector.

When OpenAI is not configured, extraction works exactly as before — no GPT calls are made.

### API

    GET /api/content/v1/parser?token=secret&url=http://aa.com/blah - extract content (emulate Readability API parse call)
    POST /api/extract {url: http://aa.com/blah}  - extract content

## Development

### Running tests

To run the full test suite, you need MongoDB running without authorisation on port 27017. To start such Mongo instance, check comments in `docker-compose.yaml` file and run Mongo according to them.

Command to run full test suite would be:

```shell
ENABLE_MONGO_TESTS=true go test ./...
```
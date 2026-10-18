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
| dbg          | DEBUG           | `false`        | debug mode                                            |

#### OpenAI (optional)

Setting `--openai.api-key` enables article summaries and selector suggestions for pages that are parsed wrong; without it no OpenAI calls are made.

| Command line              | Environment               | Default      | Description                                                        |
|---------------------------|---------------------------|--------------|--------------------------------------------------------------------|
| openai.api-key            | OPENAI_API_KEY            | none         | OpenAI API key, enables summaries and selector suggestions         |
| openai.model              | OPENAI_MODEL              | `gpt-6-luna` | OpenAI model                                                       |
| openai.summary-prompt     | OPENAI_SUMMARY_PROMPT     | built-in     | custom system prompt for summaries                                 |
| openai.max-content-length | OPENAI_MAX_CONTENT_LENGTH | `10000`      | max characters of article text sent for summarizing (0 for no limit) |

### Cloudflare Browser Rendering (optional)

Cloudflare Browser Rendering is useful for JavaScript-heavy pages and sites behind a "please enable JS" wall, but it's slower than direct HTTP and the free tier throttles at 1 request per 10 seconds. To keep the service cost-effective, Cloudflare routing is **opt-in**.

1. Set `--cf-account-id` and `--cf-api-token` to enable Cloudflare routing.
2. Either flip the `use_cloudflare` checkbox on individual rules (per-domain, recommended) or set `--cf-route-all=true` to route every request through Cloudflare.

When Cloudflare credentials are not set, the service uses a standard HTTP client for everything (default). On HTTP 429 (rate limit) the service automatically retries with exponential backoff and respects the `Retry-After` header.

### API

    GET /api/content/v1/parser?token=secret&url=http://aa.com/blah - extract content (emulate Readability API parse call)
    GET /api/content/v1/parser?token=secret&url=http://aa.com/blah&summary=true - extract content with AI-generated summary
    POST /api/extract {url: http://aa.com/blah}  - extract content
    POST /api/content-parsed-wrong?url=http://aa.com/blah - ask OpenAI for a rule for a page that is parsed wrong (credentials required)

Summary generation requires the server token to be set and an OpenAI API key. Summaries are cached in MongoDB for 30 days, keyed by model, prompt and article text; a TTL index removes expired ones. When the key is set, rule previews in the web UI also show a summary.

`POST /api/content-parsed-wrong` uses the same credentials as the rule management routes and calls OpenAI only when it is requested, never during normal extraction. The model gets an outline of the page body and suggests a CSS selector for the article. The selector is applied to the page and accepted only when it matches a plausible amount of text, not nearly the whole page and not what is extracted now; up to three suggestions are tried. The response holds the selector, the page extracted with it and `saved`. When the domain has no rule yet, the selector is stored as a **disabled** rule, so nothing changes for readers until it is reviewed and enabled in the web UI. When the domain already has a rule, enabled or not, that rule is left as it is and only the suggestion is returned.

## Development

### Running tests

To run the full test suite, you need MongoDB running without authorisation on port 27017. To start such Mongo instance, check comments in `docker-compose.yaml` file and run Mongo according to them.

Command to run full test suite would be:

```shell
ENABLE_MONGO_TESTS=true go test ./...
```

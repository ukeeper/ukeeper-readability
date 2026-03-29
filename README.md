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
| dbg          | DEBUG           | `false`        | debug mode                                            |

### Cloudflare Browser Rendering (optional)

When both `--cf-account-id` and `--cf-api-token` are set, the service uses Cloudflare Browser Rendering API to fetch page content instead of direct HTTP. This renders JavaScript and handles bot-protection pages that return empty or "just a moment..." responses to standard HTTP requests.

When these flags are not set, the service uses a standard HTTP client (default).

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
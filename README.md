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
| token        | TOKEN           | none           | token for /content/v1/parser endpoint auth            |
| mongo-delay  | MONGO_DELAY     | `0`            | mongo initial delay                                   |
| mongo-db     | MONGO_DB        | `ureadability` | mongo database name                                   |
| creds        | CREDS           | none           | credentials for protected calls (POST, DELETE /rules) |
| dbg          | DEBUG           | `false`        | debug mode                                            |

#### OpenAI integration

| Command line                  | Environment                | Default       | Description                                                      |
|-------------------------------|----------------------------|---------------|------------------------------------------------------------------|
| openai.api-key                | OPENAI_API_KEY             | none          | OpenAI API key for summary generation                            |
| openai.model-type             | OPENAI_MODEL_TYPE          | `gpt-4o-mini` | OpenAI model name (e.g., gpt-4o, gpt-4o-mini)                   |
| openai.disable-summaries      | OPENAI_DISABLE_SUMMARIES   | `false`       | disable summary generation                                       |
| openai.summary-prompt         | OPENAI_SUMMARY_PROMPT      | built-in      | custom prompt for summary generation                             |
| openai.max-content-length     | OPENAI_MAX_CONTENT_LENGTH  | `10000`       | maximum content length to send to OpenAI API (0 for no limit)    |
| openai.requests-per-minute    | OPENAI_REQUESTS_PER_MINUTE | `10`          | maximum OpenAI API requests per minute (0 for no limit)          |
| openai.cleanup-interval       | OPENAI_CLEANUP_INTERVAL    | `24h`         | interval for cleaning up expired cached summaries                |

### API

    GET /api/content/v1/parser?token=secret&url=http://aa.com/blah - extract content (emulate Readability API parse call)
    GET /api/content/v1/parser?token=secret&url=http://aa.com/blah&summary=true - extract content with AI-generated summary
    POST /api/v1/extract {url: http://aa.com/blah}  - extract content
    GET /api/metrics - summary generation metrics (cache hits, misses, response times)

Summary generation requires a valid token and an OpenAI API key. Summaries are cached in MongoDB with a 1-month expiration. Expired summaries are cleaned up automatically on the configured interval.

## Development

### Running tests

To run the full test suite, you need MongoDB running without authorisation on port 27017. To start such Mongo instance, check comments in `docker-compose.yaml` file and run Mongo according to them.

Command to run full test suite would be:

```shell
ENABLE_MONGO_TESTS=true go test ./...
```
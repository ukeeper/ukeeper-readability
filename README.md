## ukeeper-readability [![build](https://github.com/ukeeper/ukeeper-readability/actions/workflows/ci.yml/badge.svg)](https://github.com/ukeeper/ukeeper-readability/actions/workflows/ci.yml) [![Coverage Status](https://coveralls.io/repos/github/ukeeper/ukeeper-readability/badge.svg?branch=master)](https://coveralls.io/github/ukeeper/ukeeper-readability?branch=master)

### Running instructions

`docker-compose up` will leave you with working ukeeper-readability service (both API and frontend) running on <http://localhost:8080>.

### Configuration

| Command line   | Environment     | Default        | Description                                           |
|----------------|-----------------|----------------|-------------------------------------------------------|
| --address      | UKEEPER_ADDRESS | all interfaces | web server listening address                          |
| --port         | UKEEPER_PORT    | `8080`         | web server port                                       |
| --mongo-uri    | MONGO_URI       | none           | MongoDB connection string, _required_                 |
| --frontend-dir | FRONTEND_DIR    | `/srv/web`     | directory with frontend templates and static assets   |
| --token        | UKEEPER_TOKEN   | none           | token for /content/v1/parser endpoint auth            |
| --mongo-delay  | MONGO_DELAY     | `0`            | mongo initial delay                                   |
| --mongo-db     | MONGO_DB        | `ureadability` | mongo database name                                   |
| --creds        | CREDS           | none           | credentials for protected calls (POST, DELETE /rules) |
| --dbg          | DEBUG           | `false`        | debug mode                                            |

OpenAI Configuration:

| Command line                 | Environment                | Default       | Description                                                          |
|------------------------------|----------------------------|---------------|----------------------------------------------------------------------|
| --openai.disable-summaries   | OPENAI_DISABLE_SUMMARIES   | `false`       | disable summary generation with OpenAI                               |
| --openai.api-key             | OPENAI_API_KEY             | none          | OpenAI API key for summary generation                                |
| --openai.model-type          | OPENAI_MODEL_TYPE          | `gpt-4o-mini` | OpenAI model name for summary generation (e.g., gpt-4o, gpt-4o-mini) |
| --openai.summary-prompt      | OPENAI_SUMMARY_PROMPT      | *see code*    | custom prompt for summary generation                                 |
| --openai.max-content-length  | OPENAI_MAX_CONTENT_LENGTH  | `10000`       | maximum content length to send to OpenAI API                         |
| --openai.requests-per-minute | OPENAI_REQUESTS_PER_MINUTE | `10`          | maximum number of OpenAI API requests per minute                     |
| --openai.cleanup-interval    | OPENAI_CLEANUP_INTERVAL    | `24h`         | interval for cleaning up expired summaries                           |

### API

    GET /api/content/v1/parser?token=secret&summary=true&url=http://aa.com/blah - extract content (emulate Readability API parse call), summary is optional and requires OpenAI key and token to be enabled
    POST /api/v1/extract {url: http://aa.com/blah}  - extract content

### Article Summary Feature

The application can generate concise summaries of article content using OpenAI's GPT models:

1. **Configuration**:
    - Set `--openai.api-key` to your OpenAI API key
    - Summaries are enabled by default, use `--openai.disable-summaries` to disable this feature
    - Optionally set `--openai.model-type` to specify which model to use (e.g., `gpt-4o`, `gpt-4o-mini`)
        - Default is `gpt-4o-mini` if not specified
    - A server token must be configured for security reasons
    - Customize rate limiting with `--openai.requests-per-minute` (default: 10)
    - Control content length with `--openai.max-content-length` (default: 10000 characters)
    - Configure cleanup interval with `--openai.cleanup-interval` (default: 24h)

2. **Usage**:
    - Add `summary=true` parameter to the `/api/content/v1/parser` endpoint
    - Example: `/api/content/v1/parser?token=secret&summary=true&url=http://example.com/article`

3. **Features**:
    - Summaries are cached in MongoDB to reduce API costs and improve performance
    - The cache stores:
        - Content hash (to identify articles)
        - Summary text
        - Model used for generation
        - Creation and update timestamps
        - Expiration time (defaults to 1 month)
    - If the same content is requested again, the cached summary is returned
    - The preview page automatically shows summaries when available
    - Expired summaries are automatically cleaned up based on the configured interval

## Development

### Running tests

To run the full test suite, you need MongoDB running without authorisation on port 27017. To start such Mongo instance, check comments in `docker-compose.yaml` file and run Mongo according to them.

Command to run full test suite would be:

```shell
ENABLE_MONGO_TESTS=true go test ./...
```
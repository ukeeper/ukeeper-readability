## ukeeper-readability [![build](https://github.com/ukeeper/ukeeper-redabilty/actions/workflows/ci.yml/badge.svg)](https://github.com/ukeeper/ukeeper-redabilty/actions/workflows/ci.yml) [![Coverage Status](https://coveralls.io/repos/github/ukeeper/ukeeper-readability/badge.svg?branch=master)](https://coveralls.io/github/ukeeper/ukeeper-readability?branch=master)

### Running instructions

`docker-compose up` will leave you with working ukeeper-readability service (both API and frontend) running on <http://localhost:8080>.

### Configuration

| Command line | Environment     | Default        | Description                                           |
|--------------|-----------------|----------------|-------------------------------------------------------|
| address      | UKEEPER_ADDRESS | all interfaces | web server listening address                          |
| port         | UKEEPER_PORT    | `8080`         | web server port                                       |
| mongo-uri    | MONGO_URI       | none           | MongoDB connection string, _required_                 |
| api-key      | API_KEY         | none           | OpenAI API key for summary generation                 |
| model-type   | MODEL_TYPE      | `gpt-4o-mini`  | OpenAI model name for summary generation (e.g., gpt-4o, gpt-4o-mini) |
| frontend-dir | FRONTEND_DIR    | `/srv/web`     | directory with frontend files                         |
| token        | TOKEN           | none           | token for /content/v1/parser endpoint auth            |
| mongo-delay  | MONGO_DELAY     | `0`            | mongo initial delay                                   |
| mongo-db     | MONGO_DB        | `ureadability` | mongo database name                                   |
| creds        | CREDS           | none           | credentials for protected calls (POST, DELETE /rules) |
| dbg          | DEBUG           | `false`        | debug mode                                            |

### API

    GET /api/content/v1/parser?token=secret&summary=true&url=http://aa.com/blah - extract content (emulate Readability API parse call), summary is optional and requires OpenAI key and token to be enabled
    POST /api/v1/extract {url: http://aa.com/blah}  - extract content

### Article Summary Feature

The application can generate concise summaries of article content using OpenAI's GPT models:

1. **Configuration**:
   - Set `api-key` to your OpenAI API key
   - Optionally set `model-type` to specify which model to use (e.g., `gpt-4o`, `gpt-4o-mini`)
     - Default is `gpt-4o-mini` if not specified
   - A server token must be configured for security reasons

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
   - If the same content is requested again, the cached summary is returned
   - The preview page automatically shows summaries when available

## Development

### Running tests

To run the full test suite, you need MongoDB running without authorisation on port 27017. To start such Mongo instance, check comments in `docker-compose.yaml` file and run Mongo according to them.

Command to run full test suite would be:

```shell
ENABLE_MONGO_TESTS=true go test ./...
```
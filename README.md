## ukeeper-readability [![build](https://github.com/ukeeper/ukeeper-redabilty/actions/workflows/ci.yml/badge.svg)](https://github.com/ukeeper/ukeeper-redabilty/actions/workflows/ci.yml) [![Coverage Status](https://coveralls.io/repos/github/ukeeper/ukeeper-readability/badge.svg?branch=master)](https://coveralls.io/github/ukeeper/ukeeper-readability?branch=master)

### Running instructions

`docker-compose up` will leave you with working ukeeper-readability service (both API and frontend) running on <http://localhost:8080>.

### Configuration

| Command line | Environment     | Default        | Description                                           |
|--------------|-----------------|----------------|-------------------------------------------------------|
| address      | UKEEPER_ADDRESS | all interfaces | web server listening address                          |
| port         | UKEEPER_PORT    | `8080`         | web server port                                       |
| mongo_uri    | MONGO_URI       | none           | MongoDB connection string, _required_                 |
| frontend_dir | FRONTEND_DIR    | `/srv/web`     | directory with frontend files                         |
| token        | TOKEN           | none           | token for /content/v1/parser endpoint auth            |
| mongo-delay  | MONGO_DELAY     | `0`            | mongo initial delay                                   |
| mongo-db     | MONGO_DB        | `ureadability` | mongo database name                                   |
| creds        | CREDS           | none           | credentials for protected calls (POST, DELETE /rules) |
| dbg          | DEBUG           | `false`        | debug mode                                            |

### API

    GET /api/content/v1/parser?token=secret&url=http://aa.com/blah - extract content (emulate Readability API parse call)
    POST /api/v1/extract {url: http://aa.com/blah}  - extract content

    POST /api/v1/rule {"domain": "aa.com", content="#content p"} - add/update custom rule
    DELETE /api/v1/rule/:id - delete (disable) rule by ID
    GET /api/v1/rule?url=http://blah.com/aaa - get rule for url
    GET /api/v1/rules - get all rules, enabled and disabled

#### testing

on master (dev version) prefix /ureadability should be added

<details><summary>HTTP calls</summary>

    http POST "master.radio-t.com:8780/ureadability/api/v1/rule" domain=blah.ukeeper.com content="#content p" enabled:=true
    HTTP/1.1 200 OK
    Access-Control-Allow-Headers: Content-Type, Authorization, X-Requested-With
    Access-Control-Allow-Methods: GET, PUT, POST, DELETE, OPTIONS
    Access-Control-Allow-Origin: *
    Application-Name: ureadability
    Connection: keep-alive
    Content-Length: 110
    Content-Type: application/json; charset=utf-8
    Date: Mon, 11 Jan 2016 02:38:13 GMT
    Org: Umputun
    Server: nginx/1.9.7

    {
        "content": "#content p",
        "domain": "blah.ukeeper.com",
        "enabled": true,
        "id": "56931595daa6d301279ba801",
        "user": ""
    }


    http  "master.radio-t.com:8780/ureadability/api/v1/rules"
    HTTP/1.1 200 OK
    Access-Control-Allow-Headers: Content-Type, Authorization, X-Requested-With
    Access-Control-Allow-Methods: GET, PUT, POST, DELETE, OPTIONS
    Access-Control-Allow-Origin: *
    Application-Name: ureadability
    Connection: keep-alive
    Content-Length: 219
    Content-Type: application/json; charset=utf-8
    Date: Mon, 11 Jan 2016 02:38:34 GMT
    Org: Umputun
    Server: nginx/1.9.7

    [
        {
            "content": "#content p",
            "domain": "p.ukeeper.com",
            "enabled": true,
            "id": "5693123fdaa6d301279ba800",
            "user": ""
        },
        {
            "content": "#content p",
            "domain": "blah.ukeeper.com",
            "enabled": true,
            "id": "56931595daa6d301279ba801",
            "user": ""
        }
    ]

</details>

## Development

### Running tests

To run the full test suite, you need MongoDB running without authorisation on port 27017. To start such Mongo instance, check comments in `docker-compose.yaml` file and run Mongo according to them.

Command to run full test suite would be:

```shell
ENABLE_MONGO_TESTS=true go test ./...
```
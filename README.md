### ureadability api [![build](https://github.com/umputun/ukeeper-readability/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/ukeeper-readability/actions/workflows/ci.yml)


    GET /api/content/v1/parser?token=blah&url=http://aa.com/blah - extract content (emulate Readability API parse call)
    POST /api/v1/extract {url: http://aa.com/blah}  - extract content

    POST /api/v1/rule {"domain": "aa.com", content="#content p"} - add/update custom rule
	DELETE /api/v1/rule/:id - delete (disable) rule by ID
    GET /api/v1/rule?url=http://blah.com/aaa - get rule for url
	GET /api/v1/rules - get all rules, enabled and disabled


#### testing

on master (dev version) prefix /ureadability should be added


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

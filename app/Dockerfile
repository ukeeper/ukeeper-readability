# Build
FROM umputun/baseimage:buildgo-latest as build

ARG SKIP_TEST

ADD . /app
WORKDIR /app

# run tests
RUN \
    if [ -z "$SKIP_TEST" ] ; then \
        go test -timeout=30s  ./... && \
        golangci-lint run ; \
    else echo "skip tests and linter" ; fi

RUN \
    go build -o ukeeper-readability . && \
    ls -la /app/ukeeper-readability

# Run
FROM umputun/baseimage:app-latest

RUN apk add --update ca-certificates && update-ca-certificates

COPY --from=build /app/ukeeper-readability /srv/

RUN chown -R app:app /srv
USER app
WORKDIR /srv

EXPOSE 8080
CMD ["/srv/ukeeper-readability"]
ENTRYPOINT ["/init.sh", "/srv/ukeeper-readability"]
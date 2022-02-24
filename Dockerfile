# Build
FROM umputun/baseimage:buildgo-latest as build

ARG CI
ARG GITHUB_REF
ARG GITHUB_SHA
ARG GIT_BRANCH
ARG SKIP_TEST

ADD . /build
WORKDIR /build/app

# run tests and linters
RUN \
    if [ -z "$SKIP_TEST" ] ; then \
        go test -timeout=30s  ./... && \
        golangci-lint run ; \
    else echo "skip tests and linter" ; fi

RUN \
    version="$(/script/version.sh)" && \
    echo "version=$version" && \
    go build -o ukeeper-readability -ldflags "-X main.revision=${version} -s -w" .

# Run
FROM umputun/baseimage:app-latest

RUN apk add --update ca-certificates && update-ca-certificates

COPY --from=build /build/app/ukeeper-readability /srv/

RUN chown -R app:app /srv
USER app
WORKDIR /srv

EXPOSE 8080
CMD ["/srv/ukeeper-readability"]
ENTRYPOINT ["/init.sh", "/srv/ukeeper-readability"]
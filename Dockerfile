# Build
FROM umputun/baseimage:buildgo-latest as build-backend

ARG CI
ARG GITHUB_REF
ARG GITHUB_SHA
ARG GIT_BRANCH
ARG SKIP_TEST

ADD . /build
WORKDIR /build/backend

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

LABEL org.opencontainers.image.authors="Dmitry Verkhoturov <paskal.07@gmail.com>"
# enables automatic changelog generation by tools like Dependabot
LABEL org.opencontainers.image.source="https://github.com/ukeeper/ukeeper-readability"

RUN apk add --update ca-certificates && update-ca-certificates

COPY --from=build-backend /build/backend/ukeeper-readability /srv/
ADD ./backend/web /srv/web

RUN chown -R app:app /srv
USER app
WORKDIR /srv

EXPOSE 8080
CMD ["/srv/ukeeper-readability"]
ENTRYPOINT ["/init.sh", "/srv/ukeeper-readability"]
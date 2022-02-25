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

FROM node:6.17.1 as build-frontend

ADD frontend /build
WORKDIR /build

RUN \
	npm i --quiet -g gulp && \
	npm i --quiet && \
	gulp build

# Run
FROM umputun/baseimage:app-latest

RUN apk add --update ca-certificates && update-ca-certificates

COPY --from=build-backend /build/backend/ukeeper-readability /srv/
COPY --from=build-frontend /build/public /srv/web

RUN chown -R app:app /srv
USER app
WORKDIR /srv

EXPOSE 8080
CMD ["/srv/ukeeper-readability"]
ENTRYPOINT ["/init.sh", "/srv/ukeeper-readability"]
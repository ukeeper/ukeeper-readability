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

FROM node:4.9.1 as build-frontend

ENV APIPATH=/api

ADD frontend /build
WORKDIR /build

RUN \
	npm i --quiet -g gulp && \
	npm i --quiet && \
	gulp build && \
	sed -i 's|http://master.radio-t.com:8780/ureadability/api/v1|'"$APIPATH"'|g' public/js/main.js

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
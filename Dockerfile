FROM umputun/baseimage:buildgo-latest as build-backend

ARG CI
ARG GIT_BRANCH
ARG SKIP_TEST

ENV GOFLAGS="-mod=vendor"

ADD backend /build/ureadability
WORKDIR /build/ureadability

ADD .git /build/ureadability/.git

# run tests and linters
RUN \
    if [ -z "$SKIP_TEST" ] ; then \
    go test -timeout=30s  ./... && \
    golangci-lint run ; \
    else echo "skip tests and linter" ; fi

RUN \
    if [ -z "$CI" ] ; then \
    echo "runs outside of CI" && version=$(/script/git-rev.sh); \
    else version=${GIT_BRANCH}-${GITHUB_SHA:0:7}-$(date +%Y%m%dT%H:%M:%S); fi && \
    echo "version=$version" && \
    go build -o ureadability -ldflags "-X main.revision=${version} -s -w" ./app


FROM node:4.5.0 as build-frontend

ENV APIPATH=/api

ADD frontend /srv/ureadability-ui

RUN \
	cd /srv/ureadability-ui && \
	npm i --quiet -g gulp && \
	npm i --quiet && \
	gulp build && \
	rm -rf ./node_modules ./dev /tmp/* && \
	mkdir -p /var/www && \
	mv ./public /var/www/webapp && \
	sed -i 's|http://master.radio-t.com:8780/ureadability/api/v1|'"$APIPATH"'|g' /var/www/webapp/js/main.js


FROM umputun/baseimage:app-latest

COPY --from=build-backend /build/ureadability/ureadability /srv/ureadability
RUN \
    chown -R app:app /srv && \
    chmod +x /srv/ureadability

COPY --from=build-frontend /var/www/webapp /srv/web

WORKDIR /srv

CMD ["/srv/ureadability"]
ENTRYPOINT ["/init.sh"]

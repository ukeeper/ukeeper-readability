# Build
FROM umputun/baseimage:buildgo-latest AS build-backend

ARG CI
ARG GITHUB_REF
ARG GITHUB_SHA
ARG GIT_BRANCH

ADD . /build
WORKDIR /build

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

COPY --from=build-backend /build/ukeeper-readability /srv/
ADD ./web /srv/web

RUN chown -R app:app /srv
USER app
WORKDIR /srv

EXPOSE 8080
CMD ["/srv/ukeeper-readability"]
ENTRYPOINT ["/init.sh", "/srv/ukeeper-readability"]
FROM golang

ADD . /go/src/umputun.com/ureadability
ENV GO15VENDOREXPERIMENT=1
RUN \
 cd /go/src/umputun.com/ureadability && \
 go get -v && \
 GO15VENDOREXPERIMENT=1 go build -o /srv/ureadability && \
 rm -rf /go/src/*

EXPOSE 8080
ENTRYPOINT ["/srv/ureadability"]

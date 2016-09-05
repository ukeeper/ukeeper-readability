FROM golang

# add user and set TZ
RUN \
 adduser --disabled-password  --gecos "" -u 1001 umputun && \
 echo "America/Chicago" > /etc/timezone && dpkg-reconfigure -f noninteractive tzdata && \
 chown -R umputun:umputun /srv


ADD . /go/src/ukeeper.com/ureadability/app
ENV GO15VENDOREXPERIMENT=1
RUN \
 cd /go/src/ukeeper.com/ureadability/app && \
 go get -v && \
 GO15VENDOREXPERIMENT=1 go build -o /srv/ureadability && \
 rm -rf /go/src/*

EXPOSE 8080
USER umputun
WORKDIR /srv
ENTRYPOINT ["/srv/ureadability"]

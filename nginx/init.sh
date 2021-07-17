#!/bin/bash

echo "set auth users"
echo "$NGINX_ADMIN" >/etc/nginx/users
echo "$NGINX_TEST" >>/etc/nginx/users
echo >>/etc/nginx/users
cat /etc/nginx/users

exec nginx -g "daemon off;"

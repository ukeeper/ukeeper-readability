FROM nginx:1.20
ADD nginx.conf /etc/nginx/nginx.conf

ADD init.sh /init.sh
RUN chmod +x /init.sh

CMD ["/init.sh"]

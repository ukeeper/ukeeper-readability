FROM nginx:1.9
ADD nginx.conf /etc/nginx/nginx.conf

ADD init.sh /init.sh
RUN chmod +x /init.sh

CMD ["/init.sh"]

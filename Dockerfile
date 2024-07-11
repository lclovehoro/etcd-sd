FROM alpine

WORKDIR /app

ADD etcd-sd .

RUN chmod +x /app/etcd-sd \
  && cp /app/etcd-sd /usr/bin/

CMD ["etcd-sd"]

ENTRYPOINT ["etcd-sd"]

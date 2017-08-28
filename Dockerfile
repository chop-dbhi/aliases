FROM alpine:3.6

RUN apk add --no-cache ca-certificates

COPY ./dist/linux-amd64/aliases /

CMD ["/aliases", "-http", "0.0.0.0:8080", "-redis", "redis:6379"]

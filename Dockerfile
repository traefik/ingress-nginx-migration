# syntax=docker/dockerfile:1.2
# Alpine
FROM alpine

RUN apk --no-cache --no-progress add ca-certificates tzdata git \
    && rm -rf /var/cache/apk/*

ARG TARGETPLATFORM
COPY ./dist/$TARGETPLATFORM/ingress-nginx-analyzer /

ENTRYPOINT ["/ingress-nginx-analyzer"]
EXPOSE 8080

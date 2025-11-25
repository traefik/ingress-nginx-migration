# syntax=docker/dockerfile:1.2
# Alpine
FROM alpine

RUN apk --no-cache --no-progress add ca-certificates tzdata git \
    && rm -rf /var/cache/apk/*

ARG TARGETPLATFORM
COPY ./dist/$TARGETPLATFORM/ingress-nginx-migration /

ENTRYPOINT ["/ingress-nginx-migration"]
EXPOSE 8080

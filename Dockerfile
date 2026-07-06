# syntax=docker/dockerfile:1.2
# Alpine
FROM alpine:3.24.1

RUN apk --no-cache --no-progress add ca-certificates tzdata git \
    && rm -rf /var/cache/apk/*

ARG TARGETPLATFORM
COPY ./dist/$TARGETPLATFORM/ingress-nginx-migration /

USER 65534

ENTRYPOINT ["/ingress-nginx-migration"]
EXPOSE 8080

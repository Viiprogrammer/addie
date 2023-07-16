# -*- coding: utf-8 -*-
# vim: ft=Dockerfile

# container - builder
FROM golang:1.19.1-alpine AS build
LABEL maintainer="mindhunter86 <mindhunter86@vkom.cc>"
WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .
RUN set -e \
  && go build -mod=vendor -trimpath -ldflags="-s -w -X 'main.version=docker_release'" -o /anilibria-hlp-service \
  && go build -mod=vendor -trimpath -ldflags="-X 'main.version=docker_release'" -o /anilibria-hlp-service.debug

RUN apk add --no-cache upx \
  && upx -9 -k /anilibria-hlp-service \
  && apk del upx

# container - production
FROM alpine:3
LABEL maintainer="mindhunter86 <mindhunter86@vkom.cc>"
WORKDIR /

COPY --from=build --chown=root:nobody --chmod=0550 /anilibria-hlp-service /usr/local/bin/anilibria-hlp-service
COPY --from=build --chown=root:nobody --chmod=0550 /anilibria-hlp-service.debug /usr/local/bin/anilibria-hlp-service.debug

USER nobody:nobody
ENTRYPOINT ["/usr/local/bin/anilibria-hlp-service"]

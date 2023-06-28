# -*- coding: utf-8 -*-
# vim: ft=Dockerfile

FROM golang:1.19.1-alpine AS build
LABEL maintainer="mindhunter86 <mindhunter86@vkom.cc>"

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w -X 'main.version=docker_release'" -o /anilibria-hlp-service

RUN apk add --no-cache upx=4.0.2-r0 \
  && upx -9 -k /anilibria-hlp-service \
  && apk del upx


FROM alpine:3
LABEL maintainer="mindhunter86 <mindhunter86@vkom.cc>"

WORKDIR /

COPY --from=build /anilibria-hlp-service /usr/local/bin/anilibria-hlp-service

USER nobody
ENTRYPOINT ["/usr/local/bin/anilibria-hlp-service"]

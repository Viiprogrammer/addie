# -*- coding: utf-8 -*-
# vim: ft=Dockerfile

FROM golang:1.19.1-alpine AS build
LABEL maintainer="mindhunter86 <mindhunter86@vkom.cc>"

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o /anilibria-hlp-service

RUN apk add --no-cache upx \
  && upx -9 -k /anilibria-hlp-service \
  && apk del upx


FROM alpine
LABEL maintainer="mindhunter86 <mindhunter86@vkom.cc>"

WORKDIR /

COPY --from=build /anilibria-hlp-service /usr/local/bin/anilibria-hlp-service

USER nobody
ENTRYPOINT ["/usr/local/bin/anilibria-hlp-service"]

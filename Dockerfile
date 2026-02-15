# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}
COPY go.mod ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mcmm ./cmd/api

FROM docker:27-cli AS dockercli

FROM alpine:3.19
WORKDIR /app
RUN apk add --no-cache ca-certificates
RUN mkdir -p /usr/local/libexec/docker/cli-plugins
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=dockercli /usr/local/libexec/docker/cli-plugins/docker-compose /usr/local/libexec/docker/cli-plugins/docker-compose
COPY --from=builder /out/mcmm /app/mcmm
EXPOSE 8080
ENTRYPOINT ["/app/mcmm"]

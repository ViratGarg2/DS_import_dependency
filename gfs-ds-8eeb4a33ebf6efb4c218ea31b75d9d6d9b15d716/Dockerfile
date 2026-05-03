FROM golang:1.23 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gfs-master ./cmd/master
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gfs-chunkserver ./cmd/chunkserver
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gfs-client ./cmd/client

FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /out/gfs-master /usr/local/bin/gfs-master
COPY --from=builder /out/gfs-chunkserver /usr/local/bin/gfs-chunkserver
COPY --from=builder /out/gfs-client /usr/local/bin/gfs-client
COPY configs /app/configs

RUN mkdir -p /app/storage/master /app/storage/chunks

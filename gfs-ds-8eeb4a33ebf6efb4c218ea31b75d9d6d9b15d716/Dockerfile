FROM golang:1.23 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Install protoc (protobuf compiler) and ensure protoc-gen-go plugins are available
RUN apt-get update \ 
	&& apt-get install -y --no-install-recommends protobuf-compiler git ca-certificates curl \ 
	&& rm -rf /var/lib/apt/lists/*

# Make sure Go install bins are on PATH
ENV PATH="/go/bin:${PATH}"

# Install protoc Go plugins used by the Makefile to generate pb.go files
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.1 \
	&& go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0

# Generate Go protobuf sources (places .pb.go files next to .proto files)
RUN make proto

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

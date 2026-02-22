# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25.0
FROM golang:${GO_VERSION}-bookworm

ARG PROTOC_GEN_GO_VERSION=v1.36.6
ARG PROTOC_GEN_GO_GRPC_VERSION=v1.5.1

ENV GOBIN=/usr/local/bin

RUN apt-get update \
    && apt-get install -y --no-install-recommends protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION} \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}

WORKDIR /workspace

CMD ["sh", "-lc", "protoc -I . --plugin=protoc-gen-go=/usr/local/bin/protoc-gen-go --plugin=protoc-gen-go-grpc=/usr/local/bin/protoc-gen-go-grpc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative pb/order_engine/order.proto pb/order_engine/order_engine.proto"]

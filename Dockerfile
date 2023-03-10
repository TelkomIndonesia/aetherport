# syntax = docker/dockerfile:1.2

FROM golang:1.19 AS builder

WORKDIR /src
COPY ./ ./

ENV GOMODCACHE=/cache/go-mod \
    GOCACHE=/cache/go-build
RUN --mount=type=cache,target=$GOMODCACHE \
    --mount=type=cache,target=$GOCACHE \
    CGO_ENABLED=0 GOOS=linux go build -o aetherport



FROM alpine:3.16

WORKDIR /app
COPY --from=builder /src/aetherport ./aetherport
EXPOSE 8080
ENTRYPOINT ["/app/aetherport"]
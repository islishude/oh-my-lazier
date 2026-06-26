# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY . ./
RUN  --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/worker ./go/cmd/worker

FROM alpine:3.24

RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 worker

WORKDIR /app
COPY --from=build /out/worker /usr/local/bin/worker
COPY config/example.yaml /app/config/example.yaml

USER worker
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/worker"]
CMD ["-config", "/app/config/example.yaml"]

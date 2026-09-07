# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY . ./
RUN  --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o ./go/bin/ ./go/cmd/...

FROM alpine:latest

RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 worker

WORKDIR /app
COPY --from=build /src/go/bin/worker \
    /src/go/bin/txretry /src/go/bin/readinesscheck \
    /src/go/bin/configcheck /src/go/bin/draincheck  /usr/local/bin/
COPY config/example.yaml /app/config/example.yaml

USER worker
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/worker"]
CMD ["-config", "/app/config/example.yaml"]

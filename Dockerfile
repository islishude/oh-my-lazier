# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates git

ARG TARGETOS=linux
ARG TARGETARCH

COPY go/go.mod go/go.sum ./go/
WORKDIR /src/go
RUN go mod download

COPY go/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM alpine:3.23

RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 worker

WORKDIR /app
COPY --from=build /out/worker /usr/local/bin/worker
COPY config/example.yaml /app/config/example.yaml

USER worker
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/worker"]
CMD ["-config", "/app/config/example.yaml"]

# syntax=docker/dockerfile:1
ARG PCS_IMAGE=ghcr.io/kyxie/baidupcs-go:v4.0.2
FROM --platform=$BUILDPLATFORM golang:1.27.4-bookworm AS go-base
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

FROM go-base AS verify
RUN test -z "$(gofmt -l cmd internal)" \
    && go vet ./... \
    && go test -race ./... \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/ingest ./cmd/ingest

FROM go-base AS dev
RUN go install github.com/air-verse/air@v1.67.4
CMD ["air", "-c", ".air.toml"]

FROM go-base AS build
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w' -o /out/ingest ./cmd/ingest

FROM ${PCS_IMAGE} AS pcs

FROM alpine:3.24 AS runtime
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/ingest /ingest
COPY --from=pcs /usr/local/bin/BaiduPCS-Go /usr/local/bin/BaiduPCS-Go
ENTRYPOINT ["/ingest"]
CMD ["scan"]

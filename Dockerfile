# syntax=docker/dockerfile:1
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

FROM --platform=$BUILDPLATFORM golang:1.27.4-bookworm AS pcsbuild
ARG PCS_VERSION=v4.0.2
ARG TARGETOS
ARG TARGETARCH
RUN git clone --depth 1 --branch "$PCS_VERSION" https://github.com/qjfoidnh/BaiduPCS-Go.git /pcs
WORKDIR /pcs
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w' -o /out/BaiduPCS-Go .

FROM alpine:3.24 AS runtime
RUN apk add --no-cache ca-certificates tzdata

FROM scratch AS importer
COPY --from=build /out/ingest /ingest
ENTRYPOINT ["/ingest"]
CMD ["scan"]

FROM runtime AS uploader
COPY --from=build /out/ingest /ingest
COPY --from=pcsbuild /out/BaiduPCS-Go /usr/local/bin/BaiduPCS-Go
ENTRYPOINT ["/ingest"]
CMD ["upload"]

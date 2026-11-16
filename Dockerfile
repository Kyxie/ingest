# syntax=docker/dockerfile:1
FROM golang:1.27.4-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/ingest ./cmd/ingest

FROM golang:1.27.4-bookworm AS pcsbuild
ARG PCS_VERSION=v4.0.2
RUN git clone --depth 1 --branch "$PCS_VERSION" https://github.com/qjfoidnh/BaiduPCS-Go.git /pcs
WORKDIR /pcs
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/BaiduPCS-Go .

FROM scratch AS importer
COPY --from=build /out/ingest /ingest
ENTRYPOINT ["/ingest"]
CMD ["scan"]

FROM alpine:3.24 AS uploader
RUN apk add --no-cache ca-certificates
COPY --from=build /out/ingest /ingest
COPY --from=pcsbuild /out/BaiduPCS-Go /usr/local/bin/BaiduPCS-Go
ENTRYPOINT ["/ingest"]
CMD ["upload"]

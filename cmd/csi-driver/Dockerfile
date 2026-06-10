# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /csi-driver ./cmd/csi-driver

# ---- runtime stage ----
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    fuse3 git ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=build /csi-driver /usr/local/bin/csi-driver

ENTRYPOINT ["/usr/local/bin/csi-driver", "--csi-address=unix:///csi/csi.sock", "--root=/var/lib/artifact-fs-csi"]

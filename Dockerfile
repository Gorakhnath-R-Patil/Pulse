# syntax=docker/dockerfile:1
#
# Builds all three Pulse binaries into one image. Select which to run with
# the container command, e.g. `pulse-agent` or `pulse-collector`.
#
# The eBPF bindings are not committed (see ADR-007 in
# docs/design/decisions.md), so the build stage installs clang and runs
# `go generate` before compiling — the same steps CI performs.

FROM golang:1.25-bookworm AS build
RUN apt-get update \
    && apt-get install -y --no-install-recommends clang \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go generate ./...

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
ENV CGO_ENABLED=0
RUN set -eux; \
    MODULE=github.com/Gorakhnath-R-Patil/Pulse; \
    LDFLAGS="-s -w \
      -X ${MODULE}/internal/version.Version=${VERSION} \
      -X ${MODULE}/internal/version.Commit=${COMMIT} \
      -X ${MODULE}/internal/version.BuildDate=${BUILD_DATE}"; \
    for cmd in pulse-agent pulse-collector pulse-cli; do \
      go build -ldflags "${LDFLAGS}" -o /out/${cmd} ./cmd/${cmd}; \
    done

# Static binaries, so a distroless static base is enough. This image runs as
# root by default because pulse-agent needs to load eBPF programs; the
# collector does not, and the Helm chart runs it as a non-root user.
FROM gcr.io/distroless/static-debian12:latest
COPY --from=build /out/pulse-agent /out/pulse-collector /out/pulse-cli /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/pulse-agent"]

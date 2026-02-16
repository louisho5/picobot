# syntax=docker/dockerfile:1.7

# ---- Build Stage ----
ARG BUILDPLATFORM=linux/amd64
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN set -eux; \
    GOOS="${TARGETOS:-linux}"; \
    GOARCH="${TARGETARCH:-amd64}"; \
    if [ "$GOARCH" = "arm" ] && [ -n "${TARGETVARIANT}" ]; then \
        GOARM="${TARGETVARIANT#v}"; \
    fi; \
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" ${GOARM:+GOARM=$GOARM} \
    go build -ldflags="-s -w" -o /picobot ./cmd/picobot

# ---- Runtime Stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata bash curl su-exec

ENV PICOBOT_USER=picobot \
    PICOBOT_GROUP=picobot \
    PICOBOT_HOME=/home/picobot/.picobot

# Create default non-root user (can be remapped at runtime)
RUN addgroup -g 101 -S ${PICOBOT_GROUP} && adduser -S -u 100 -G ${PICOBOT_GROUP} ${PICOBOT_USER}

# Picobot home directory (config + workspace live here)
RUN mkdir -p ${PICOBOT_HOME} && chown -R ${PICOBOT_USER}:${PICOBOT_GROUP} /home/picobot

COPY --from=builder /picobot /usr/local/bin/picobot
RUN chmod +x /usr/local/bin/picobot

WORKDIR /home/picobot

# Run onboard on first start to create config + workspace,
# then start the gateway. The entrypoint script handles this.
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER root

ENTRYPOINT ["/entrypoint.sh"]
CMD ["gateway"]

# syntax=docker/dockerfile:1.7
#
# One image, four static binaries: /app/api (default), /app/worker, /app/migrate, /app/telegram.
#
#   docker build -t socialos-backend .
#   docker run --env-file .env -p 8080:8080 socialos-backend                  # API
#   docker run --env-file .env socialos-backend /app/worker                   # worker
#   docker run --env-file .env socialos-backend /app/migrate up               # migrations
#   docker run --env-file .env socialos-backend /app/telegram set-webhook     # Telegram webhook mode only
#
# The runtime stage is distroless "static:nonroot": no shell, no package manager,
# uid/gid 65532. HEALTHCHECK uses `api healthcheck` because there is no curl.

ARG GO_VERSION=1.24

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
# Dependencies first so they stay cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download && go mod verify
COPY . .
ENV CGO_ENABLED=0 GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    for cmd in api worker migrate telegram; do \
      go build -trimpath -ldflags="-s -w" -o "/out/${cmd}" "./cmd/${cmd}"; \
    done

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app
COPY --from=build /out/api /out/worker /out/migrate /out/telegram /app/
# Migrations are embedded in the binaries; nothing else is needed at runtime.
USER 65532:65532
ENV APP_ENV=production HTTP_ADDR=:8080 LOG_FORMAT=json
EXPOSE 8080 8081
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD ["/app/api", "healthcheck"]
CMD ["/app/api"]

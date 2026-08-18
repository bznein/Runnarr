FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS api-build
WORKDIR /src
RUN apk add --no-cache ca-certificates
ARG RUNNARR_BUILD_VERSION=dev
ARG RUNNARR_BUILD_COMMIT=unknown
ARG RUNNARR_MIGRATION_HASH=unknown
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build \
  -ldflags "-s -w \
    -X github.com/bznein/Runnarr/internal/app.BuildVersion=${RUNNARR_BUILD_VERSION} \
    -X github.com/bznein/Runnarr/internal/app.BuildCommit=${RUNNARR_BUILD_COMMIT} \
    -X github.com/bznein/Runnarr/internal/app.BuildMigrationHash=${RUNNARR_MIGRATION_HASH}" \
  -o /out/runnarr ./cmd/runnarr

FROM python:3.13-slim
ARG RUNNARR_BUILD_VERSION=dev
ARG RUNNARR_BUILD_COMMIT=unknown
ARG RUNNARR_MIGRATION_HASH=unknown
LABEL org.opencontainers.image.source="https://github.com/bznein/Runnarr" \
  org.opencontainers.image.version="${RUNNARR_BUILD_VERSION}" \
  org.opencontainers.image.revision="${RUNNARR_BUILD_COMMIT}" \
  com.runnarr.migrations="${RUNNARR_MIGRATION_HASH}"
WORKDIR /app
RUN apt-get update \
  && apt-get upgrade -y \
  && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/* \
  && pip install --no-cache-dir \
    'garminconnect[workout]==0.3.8' \
    'msgpack==1.2.1' \
  && pip uninstall -y pip setuptools \
  && rm -rf /usr/local/lib/python3.13/ensurepip
COPY --from=api-build /out/runnarr /app/runnarr
COPY --from=web-build /src/web/dist /app/web/dist
COPY internal/app/garmin_bridge.py /app/garmin_bridge.py
COPY internal/app/garmin_bridge_testbed.py /app/garmin_bridge_testbed.py
RUN addgroup --system --gid 10001 runnarr \
  && adduser --system --uid 10001 --gid 10001 --home /app runnarr \
  && mkdir -p /app/data \
  && chown -R 10001:10001 /app \
  && chmod 700 /app/data
ENV PYTHONDONTWRITEBYTECODE=1
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/app/runnarr"]

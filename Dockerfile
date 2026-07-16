FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS api-build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/runnarr ./cmd/runnarr

FROM python:3.13-slim
WORKDIR /app
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/* \
  && pip install --no-cache-dir garminconnect curl_cffi
COPY --from=api-build /out/runnarr /app/runnarr
COPY --from=web-build /src/web/dist /app/web/dist
COPY internal/app/garmin_bridge.py /app/garmin_bridge.py
RUN mkdir -p /app/data && chmod 700 /app/data
EXPOSE 8080
ENTRYPOINT ["/app/runnarr"]

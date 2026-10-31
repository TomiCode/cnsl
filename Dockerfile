# syntax=docker/dockerfile:1

# --- dashboard (Vue) ---
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/yarn.lock ./
RUN yarn install --frozen-lockfile
COPY api /src/api
COPY web ./
RUN yarn build

# --- server (Go, no CGO so it cross-compiles for any TARGETARCH) ---
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/cnsl ./cmd/cnsl \
 && mkdir -p /out/data

# --- runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/cnsl /cnsl
COPY --from=build --chown=65532:65532 /out/data /data
ENV CNSL_DATA_DIR=/data \
    CNSL_LOG_FORMAT=json
VOLUME /data
EXPOSE 8080 8081
ENTRYPOINT ["/cnsl"]

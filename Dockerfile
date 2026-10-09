# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:22-slim AS ui
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
COPY --from=ui /web/dist ./web/dist
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/appdata-suppressor .

# Alpine only for busybox wget in the healthcheck. The app talks to the host
# daemon through the mounted /var/run/docker.sock, no docker CLI needed.
FROM alpine:3.20
COPY --from=build /out/appdata-suppressor /usr/local/bin/appdata-suppressor

EXPOSE 8765
HEALTHCHECK --interval=30s --timeout=5s \
    CMD wget -q -O /dev/null http://127.0.0.1:8765/ || exit 1

ENTRYPOINT ["appdata-suppressor"]

# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
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
    CMD wget -q -O /dev/null http://127.0.0.1:8765/login.html || exit 1

ENTRYPOINT ["appdata-suppressor"]

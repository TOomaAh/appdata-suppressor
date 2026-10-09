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

FROM alpine:3.20
# The scanner shells out to the docker CLI, which talks to the host daemon
# through the mounted /var/run/docker.sock.
RUN apk add --no-cache docker-cli
COPY --from=build /out/appdata-suppressor /usr/local/bin/appdata-suppressor

EXPOSE 8765
HEALTHCHECK --interval=30s --timeout=5s \
    CMD wget -q -O /dev/null http://127.0.0.1:8765/login.html || exit 1

ENTRYPOINT ["appdata-suppressor"]

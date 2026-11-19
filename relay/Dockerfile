# syntax=docker/dockerfile:1

# Cross-compiles on the build machine for each target platform, no emulation needed.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/webhook-outbox-relay ./cmd/webhook-outbox-relay

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/webhook-outbox-relay /webhook-outbox-relay
# Numeric, so Kubernetes can check runAsNonRoot without knowing the name.
USER 65532:65532
ENTRYPOINT ["/webhook-outbox-relay"]

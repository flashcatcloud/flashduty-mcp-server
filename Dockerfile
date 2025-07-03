FROM golang:1.24.4-alpine AS build
ARG VERSION="dev"
ARG TARGETARCH

# Set the working directory
WORKDIR /build

# Install git
RUN --mount=type=cache,target=/var/cache/apk \
    apk add git

# Build the server
# go build automatically download required module dependencies to /go/pkg/mod
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOARCH=${TARGETARCH} go build -ldflags="-s -w -X main.version=${VERSION} -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /bin/flashduty-mcp-server cmd/flashduty-mcp-server/main.go

# Make a stage to run the app
FROM gcr.io/distroless/base-debian12
# Set the working directory
WORKDIR /server
# Copy the binary from the build stage
COPY --from=build /bin/flashduty-mcp-server .

# Set environment variables for Flashduty
ENV FLASHDUTY_APP_KEY=""
ENV FLASHDUTY_BASE_URL="https://api.flashcat.cloud"
ENV FLASHDUTY_READ_ONLY=""
ENV FLASHDUTY_TOOLSETS=""
ENV FLASHDUTY_LOG_FILE=""
ENV FLASHDUTY_ENABLE_COMMAND_LOGGING=""

# Set the entrypoint to the server binary
ENTRYPOINT ["/server/flashduty-mcp-server"]
# Default arguments for ENTRYPOINT
CMD ["stdio"]

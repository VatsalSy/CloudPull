# Build stage
FROM golang:1.21.13-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git=2.43.6-r0 make=4.4.1-r2

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN make build

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates=20241121-r1 tzdata=2025b-r0

# Create non-root user and necessary directories
RUN adduser -D -u 1000 cloudpull && \
    mkdir -p /home/cloudpull/.cloudpull && \
    chown -R cloudpull:cloudpull /home/cloudpull

# Copy binary from builder
COPY --from=builder /build/build/cloudpull /usr/local/bin/cloudpull

# Switch to non-root user
USER cloudpull

# Set working directory
WORKDIR /home/cloudpull

# Add health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD cloudpull version || exit 1

# Default data directory
VOLUME ["/data", "/home/cloudpull/.cloudpull"]

# Entry point
ENTRYPOINT ["cloudpull"]

# Default command (show help)
CMD ["--help"]

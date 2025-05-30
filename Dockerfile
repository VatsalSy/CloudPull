# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

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
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -u 1000 cloudpull

# Create necessary directories
RUN mkdir -p /home/cloudpull/.cloudpull && \
    chown -R cloudpull:cloudpull /home/cloudpull

# Copy binary from builder
COPY --from=builder /build/build/cloudpull /usr/local/bin/cloudpull

# Switch to non-root user
USER cloudpull

# Set working directory
WORKDIR /home/cloudpull

# Default data directory
VOLUME ["/data", "/home/cloudpull/.cloudpull"]

# Entry point
ENTRYPOINT ["cloudpull"]

# Default command (show help)
CMD ["--help"]
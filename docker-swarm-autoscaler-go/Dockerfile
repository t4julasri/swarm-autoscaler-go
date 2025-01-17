# Build stage
FROM golang:1.23 AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod ./
COPY go.* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o autoscaler

# Final stage
FROM ubuntu:jammy

# Install required packages
RUN apt-get update -qq && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    docker.io && \
    apt-get clean && \
    apt-get autoremove -y && \
    rm -rf \
    /var/lib/apt/lists/* \
    /tmp/* \
    /var/tmp/*

# Copy binary from builder
COPY --from=builder /app/autoscaler /usr/local/bin/autoscaler

# Make binary executable
RUN chmod +x /usr/local/bin/autoscaler

# Set environment variables with defaults
ENV PROMETHEUS_URL=""
ENV CHECK_INTERVAL="60"
ENV CPU_MIN="25"
ENV CPU_MAX="85"

# Create directory for Docker socket
VOLUME /var/run

CMD ["autoscaler"]
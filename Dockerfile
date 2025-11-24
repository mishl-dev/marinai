# --------------------
# Builder stage
# --------------------
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

# Install only what is needed for building
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy Go modules first to leverage caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build statically with optimizations
# TARGETARCH is automatically provided by buildx
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o marinai ./main.go

# --------------------
# Final minimal image
# --------------------
FROM --platform=$TARGETPLATFORM gcr.io/distroless/base

# Set working directory
WORKDIR /app

# Copy CA certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary from builder
COPY --from=builder /build/marinai .

# Use non-root user (numeric UID 1000)
USER 1000

# Entrypoint
ENV PATH="/app:${PATH}"
CMD ["./marinai"]

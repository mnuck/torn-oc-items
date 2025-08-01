# Build stage - Use latest 1.24.2 Alpine image
FROM golang:1.24.2-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download and verify dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application with build info and security flags
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a \
    -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o torn-oc-items \
    .

# Final stage - Use latest distroless static image
FROM gcr.io/distroless/static:nonroot

# Copy binary from builder stage
COPY --from=builder /app/torn-oc-items /app/torn-oc-items

WORKDIR /app

# Use built-in nonroot user (UID 65532)
USER 65532:65532

# Set default command
CMD ["./torn-oc-items"]
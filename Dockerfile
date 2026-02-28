# Build stage - Use latest 1.26.0 Alpine image
FROM --platform=$BUILDPLATFORM golang:1.26.0-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download and verify dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application with build info and security flags
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
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
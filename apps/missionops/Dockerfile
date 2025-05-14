FROM golang:1.24 AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod .

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o missionops .

# Create minimal image from scratch
FROM alpine:latest

# Install CA certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Create data directory
RUN mkdir -p /app/data

# Copy binary from builder
COPY --from=builder /app/missionops .

# Set execute permissions
RUN chmod +x /app/missionops

EXPOSE 8087

CMD ["/app/missionops"]
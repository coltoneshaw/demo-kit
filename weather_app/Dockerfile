FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o weather-app .

# Use a smaller image for the final container
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS requests and Docker client
RUN apk --no-cache add ca-certificates docker-cli

# Copy the binary from the builder stage
COPY --from=builder /app/weather-app .

# Create directory for static files
RUN mkdir -p /app/static

# Copy static files
COPY static/ /app/static/

# Expose the port the app runs on
EXPOSE 8085

# Command to run the application
CMD ["./weather-app"]

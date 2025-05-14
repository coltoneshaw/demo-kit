FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod ./
COPY go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Create an empty subscriptions file if it doesn't exist
RUN touch flight_subscriptions.json

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o flightaware-app .

# Use a small alpine image for the final container
FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/flightaware-app .
COPY --from=builder /app/flight_subscriptions.json .

# Create data directory for persistence
RUN mkdir -p /app/data

# Expose the port the app runs on
EXPOSE 8086

# Set environment variables
ENV PORT=8086

# Command to run the application
CMD ["./flightaware-app"]

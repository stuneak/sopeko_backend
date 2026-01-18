# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install git for fetching dependencies
RUN apk update && apk add --no-cache git curl

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Production stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates, tzdata, and migrate CLI
RUN apk --no-cache add ca-certificates tzdata curl && \
    curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xz && \
    mv migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate

# Copy binary from builder
COPY --from=builder /app/main .

# Copy migrations
COPY --from=builder /app/db/sqlc/migration ./db/sqlc/migration

# Copy entrypoint
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh

# Expose port
EXPOSE 8080
# Run migrations and start the application
ENTRYPOINT ["./entrypoint.sh"]

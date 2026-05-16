# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies for CGO if needed
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server/main.go

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies:
# - ca-certificates: for HTTPS calls
# - poppler-utils: for PDF text extraction (pdftotext, pdfinfo)
RUN apk --no-cache add ca-certificates poppler-utils

# Copy binary from builder
COPY --from=builder /app/server .

# Create data directories
RUN mkdir -p /app/data/documents /app/data/parsed /app/data/state

# Expose port
EXPOSE 8080

# Set environment variables
ENV PORT=8080
ENV DATA_DIR=/app/data
ENV OLLAMA_HOST=http://ollama:11434
ENV MODEL_NAME=llama3

# Run the binary
CMD ["./server"]

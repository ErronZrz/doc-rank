# --- Build stage ---
FROM golang:1.24 AS builder

WORKDIR /app
COPY go.mod ./
COPY . .
RUN go mod download
RUN go build -o server ./cmd/main.go

# Run stage
FROM gcr.io/distroless/base-debian11
WORKDIR /app
COPY --from=builder /app/server .
COPY .env .

EXPOSE 8080
CMD ["./server"]

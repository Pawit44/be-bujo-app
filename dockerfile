# --- Build stage ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Cache go modules separately for faster rebuilds
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 for a static binary that runs on the minimal alpine image below
RUN CGO_ENABLED=0 GOOS=linux go build -o server main.go

# --- Run stage ---
FROM alpine:3.20

# certs needed for HTTPS calls (e.g. to Supabase)
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/server .

# Render injects PORT at runtime; app must read it from env (already does via PORT var)
EXPOSE 8080

CMD ["./server"]
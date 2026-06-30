# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binary — no libc dependency, strips debug symbols
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -extldflags=-static" \
    -o /quell \
    ./cmd/quell

# ── Stage 2: minimal runtime ───────────────────────────────────────────────────
FROM alpine:3.20

# ca-certificates needed for HTTPS calls to IBM/AWS/Google
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /workspace

COPY --from=builder /quell /usr/local/bin/quell

ENTRYPOINT ["quell"]
CMD ["serve"]

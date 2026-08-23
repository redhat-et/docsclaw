FROM golang:1.26.6-alpine3.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /docsclaw ./cmd/docsclaw

FROM alpine:3.24

RUN apk --no-cache add ca-certificates curl ripgrep && \
    addgroup -S docsclaw && adduser -S docsclaw -G docsclaw

WORKDIR /app
COPY --from=builder /docsclaw /app/docsclaw

USER docsclaw

EXPOSE 8000

ENTRYPOINT ["/app/docsclaw"]
CMD ["serve"]

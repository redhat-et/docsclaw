FROM registry.access.redhat.com/hi/go:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /docsclaw ./cmd/docsclaw

FROM registry.access.redhat.com/hi/core-runtime:latest

WORKDIR /app
COPY --from=builder /docsclaw /app/docsclaw

EXPOSE 8000

ENTRYPOINT ["/app/docsclaw"]
CMD ["serve"]

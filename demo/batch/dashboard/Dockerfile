FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o dashboard .

FROM scratch
COPY --from=builder /build/dashboard /dashboard
USER 65534:65534
EXPOSE 8090
ENTRYPOINT ["/dashboard"]

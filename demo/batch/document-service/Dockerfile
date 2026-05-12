FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY main.go .
RUN CGO_ENABLED=0 go build -o document-service main.go

FROM scratch
COPY --from=builder /build/document-service /document-service
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/document-service"]
CMD ["-data-dir", "/data"]

FROM registry.access.redhat.com/hi/go:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /docsclaw ./cmd/docsclaw

FROM registry.access.redhat.com/hi/core-runtime:latest

# Adding tools to the minimal hardened image expands its attack surface.
# Only add what is strictly necessary for runtime operation (e.g. health
# checks). Review each addition with your security team.
USER root
RUN --mount=type=bind,from=registry.access.redhat.com/hi/core-runtime:latest-builder,target=/builder \
    LD_LIBRARY_PATH=/builder/lib64:/builder/usr/lib64 \
    RPM_CONFIGDIR=/builder/usr/lib/rpm \
    /builder/usr/bin/dnf install -y \
    --installroot=/ \
    --setopt=reposdir=/builder/etc/yum.repos.d \
    --setopt=install_weak_deps=False \
    --setopt=tsflags=nodocs \
    curl jq
USER 65532

WORKDIR /app
COPY --from=builder /docsclaw /app/docsclaw

EXPOSE 8000

ENTRYPOINT ["/app/docsclaw"]
CMD ["serve"]

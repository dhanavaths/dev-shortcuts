FROM mcr.microsoft.com/oss/go/microsoft/golang:1.22 AS builder
WORKDIR /src
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/discovery ./cmd/discovery && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/router    ./cmd/router

# Final image includes `tc` (iproute2) and `iptables` so the same
# image can be used as the netem sidecar.
FROM alpine:3.20
RUN apk add --no-cache iproute2-tc iptables bash curl && \
    curl -sSL -o /usr/local/bin/kubectl \
      https://dl.k8s.io/release/v1.30.0/bin/linux/amd64/kubectl && \
    chmod +x /usr/local/bin/kubectl
COPY --from=builder /out/discovery /usr/local/bin/discovery
COPY --from=builder /out/router    /usr/local/bin/router
COPY scripts/netem.sh /usr/local/bin/netem.sh
RUN chmod +x /usr/local/bin/netem.sh

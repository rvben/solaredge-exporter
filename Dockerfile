FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /solaredge-exporter .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
LABEL org.opencontainers.image.source=https://github.com/rvben/solaredge-exporter
LABEL org.opencontainers.image.description="Prometheus exporter for SolarEdge inverters"
LABEL org.opencontainers.image.licenses=MIT
COPY --from=builder /solaredge-exporter /usr/local/bin/solaredge-exporter
EXPOSE 2112
ENTRYPOINT ["solaredge-exporter"]

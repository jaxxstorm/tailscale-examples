# syntax=docker/dockerfile:1

FROM cgr.dev/chainguard/go:latest AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

# Copy the remaining project files
COPY cmd/server .

RUN CGO_ENABLED=0 GOOS=linux go build -o /src/envoy-server .

FROM cgr.dev/chainguard/static:latest

# Default to port 8080, but it can be overridden at runtime
ENV SERVER_PORT=8080

# Copy the binary from the builder stage
COPY --from=builder /src/envoy-server /usr/local/bin/envoy-server

EXPOSE 8080

# Launch
ENTRYPOINT ["/usr/local/bin/envoy-server"]
FROM golang as builder

WORKDIR /app

COPY main.go /app/main.go
COPY go.mod /app/go.mod
RUN go mod tidy
RUN go build -o /app/example main.go

FROM golang
WORKDIR /app
COPY --from=builder /app/example /app
EXPOSE 50051
CMD ["./example"]
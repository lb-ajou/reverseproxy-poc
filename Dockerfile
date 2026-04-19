FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY main.go ./
COPY configs ./configs
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/reverseproxy ./main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /out/reverseproxy /app/reverseproxy
COPY configs /app/configs

EXPOSE 8080 9090

ENTRYPOINT ["/app/reverseproxy"]

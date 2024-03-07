FROM golang:1.20-alpine AS builder
RUN apk add --no-cache build-base
WORKDIR /app/src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go hash-alert.html height-alert.html ./
RUN go build -o go-xpx-check-fork-util

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/src/go-xpx-check-fork-util .
COPY --from=builder /app/src/hash-alert.html .
COPY --from=builder /app/src/height-alert.html .

ENTRYPOINT ["./go-xpx-check-fork-util"]
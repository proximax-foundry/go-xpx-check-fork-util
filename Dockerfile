FROM golang:1.20-alpine AS builder
RUN apk add --no-cache build-base
WORKDIR /app/src
COPY main.go \
  go.mod \
  go.sum \
  hash-alert.html \
  height-alert.html \
  ./
RUN go mod init go-xpx-check-fork-util
RUN go mod tidy
RUN go build -o go-xpx-check-fork-util

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/src/go-xpx-check-fork-util .
COPY --from=builder /app/src/hash-alert.html .
COPY --from=builder /app/src/height-alert.html .

ENTRYPOINT ["./go-xpx-check-fork-util"]
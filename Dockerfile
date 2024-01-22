FROM golang:1.20-alpine AS builder
RUN apk add --no-cache build-base
WORKDIR /app/src
COPY main.go \
  go.mod \
  go.sum \
  ./
RUN go mod init go-xpx-check-fork-util
RUN go mod tidy
RUN go build -o go-xpx-check-fork-util

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/src/go-xpx-check-fork-util .

ENTRYPOINT ["./go-xpx-check-fork-util"]
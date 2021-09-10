FROM golang:1.17 as build

WORKDIR /app
COPY . . 
RUN go mod download
RUN go build -o go-xpx-check-fork-util main.go

FROM golang:1.17
WORKDIR /app
COPY --from=build /app/go-xpx-check-fork-util .
COPY --from=build /app/config.json .
CMD ["/app/go-xpx-check-fork-util"]


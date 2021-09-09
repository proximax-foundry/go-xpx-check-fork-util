FROM golang:1.17 as build

WORKDIR /app
COPY . . 
RUN go mod download
RUN go build -o go-xpx-check-fork-util script.go

FROM golang:1.17
WORKDIR /app
COPY --from=build /app/go-xpx-check-for-util .
CMD ["/app/go-xpx-check-for-util"]


FROM golang:1.19 as builder
WORKDIR /echo

COPY go.mod go.mod

RUN go mod download

COPY main.go main.go

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o echo main.go

FROM gcr.io/distroless/static-debian11:nonroot

WORKDIR /echo
COPY --from=builder /echo/echo .
USER nonroot

ENTRYPOINT ["/echo/echo"]
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /website-audit .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /website-audit /usr/bin/website-audit
ENTRYPOINT ["/usr/bin/website-audit"]

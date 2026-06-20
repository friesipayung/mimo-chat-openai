FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /mimo-proxy ./cmd/mimo-proxy

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /mimo-proxy /usr/local/bin/mimo-proxy

RUN mkdir -p /data

VOLUME /data
EXPOSE 8090

CMD ["mimo-proxy"]

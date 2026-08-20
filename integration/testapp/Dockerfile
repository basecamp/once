FROM golang:1.26.7 AS builder

WORKDIR /app

COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -o testapp .

FROM debian:stable-slim

LABEL org.opencontainers.image.source=https://github.com/basecamp/once

COPY --from=builder /app/testapp /testapp

RUN mkdir -p /storage /rails/storage /hooks && \
    chown 1000:1000 /storage /rails/storage

USER 1000:1000

EXPOSE 80

CMD ["/testapp"]

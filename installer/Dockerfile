FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download || true

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o installer .

FROM scratch

COPY --from=builder /app/installer /installer

EXPOSE 80

ENTRYPOINT ["/installer"]

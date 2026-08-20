FROM --platform=$BUILDPLATFORM golang:1.26.7 AS builder
ARG TARGETARCH

WORKDIR /app

COPY go.mod ./
RUN go mod download || true

COPY . .
RUN CGO_ENABLED=0 GOARCH=$TARGETARCH go build -trimpath -o installer .

FROM scratch

COPY --from=builder /app/installer /installer

EXPOSE 80

ENTRYPOINT ["/installer"]

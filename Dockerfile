FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ . ./cmd/periodicsync

FROM debian:bookworm-slim AS periodicsync
RUN apt-get update && apt-get install -y --no-install-recommends poppler-utils ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/periodicsync /app/periodicsync
COPY web/who /app/web/who
ENTRYPOINT ["/app/periodicsync"]

FROM gcr.io/distroless/static-debian12 AS serve
WORKDIR /app
COPY --from=build /out/heliosian /app/heliosian
COPY web /app/web
ENTRYPOINT ["/app/heliosian"]

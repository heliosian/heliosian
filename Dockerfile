FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ .

FROM gcr.io/distroless/static-debian12 AS serve
WORKDIR /app
COPY --from=build /out/heliosian /app/heliosian
COPY web /app/web
ENTRYPOINT ["/app/heliosian"]

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /heliosian .

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /heliosian /app/heliosian
COPY web /app/web
ENTRYPOINT ["/app/heliosian"]

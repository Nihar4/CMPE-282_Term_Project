FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o api-gateway .

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /app/api-gateway .
EXPOSE 8080
ENTRYPOINT ["/app/api-gateway"]

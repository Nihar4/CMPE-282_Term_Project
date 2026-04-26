FROM golang:1.21-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o data-service .

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /app/data-service .
EXPOSE 8082
ENTRYPOINT ["/app/data-service"]

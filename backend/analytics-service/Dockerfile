FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o analytics-service .

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /app/analytics-service .
EXPOSE 8085
ENTRYPOINT ["/app/analytics-service"]

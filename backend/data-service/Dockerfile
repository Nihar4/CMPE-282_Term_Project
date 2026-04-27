FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o data-service .

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /app/data-service .
EXPOSE 8082
ENTRYPOINT ["/app/data-service"]

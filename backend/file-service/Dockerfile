FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o file-service .

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /app/file-service .
EXPOSE 8083
ENTRYPOINT ["/app/file-service"]

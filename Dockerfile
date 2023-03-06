# Build environment
FROM golang:1.19-alpine AS build-env
RUN apk add build-base
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o app cmd/main.go

# Run environment
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-env /app/app .
EXPOSE 8080
CMD ["./app"]
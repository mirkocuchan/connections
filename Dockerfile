# ---- compilar ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server .

# ---- ejecutar ----
FROM alpine:3.20
# ca-certificates: para llamar por HTTPS a Resend y a Expo Push
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/server /app/server
USER app
EXPOSE 8080
CMD ["/app/server"]

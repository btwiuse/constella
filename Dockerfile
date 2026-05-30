FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /tmp/constella ./cmd/constella

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /tmp/constella /bin/constella
CMD ["constella"]

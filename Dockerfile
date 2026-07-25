FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/strata-rmm .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/strata-rmm /usr/local/bin/strata-rmm
RUN mkdir -p /var/lib/strata-rmm /etc/strata-rmm
EXPOSE 8080
ENTRYPOINT ["strata-rmm"]
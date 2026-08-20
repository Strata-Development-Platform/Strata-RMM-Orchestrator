FROM golang:1.26.6-alpine AS builder
RUN apk add --no-cache gcc musl-dev git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG STRATA_VERSION=0.0.0-dev
ARG STRATA_COMMIT=none
ARG STRATA_BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${STRATA_VERSION} -X main.commit=${STRATA_COMMIT} -X main.date=${STRATA_BUILD_DATE}" \
    -o /bin/strata-rmm .
RUN sh scripts/build-cosign-verifier.sh /tmp/cosign-verifier

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata docker-cli docker-cli-compose postgresql-client
COPY --from=builder /bin/strata-rmm /usr/local/bin/strata-rmm
COPY --from=builder /tmp/cosign-verifier/cosign /usr/local/bin/cosign
COPY --from=builder /tmp/cosign-verifier/BUILD-PROVENANCE.txt /usr/share/strata-rmm/cosign-verifier-provenance.txt
RUN mkdir -p /var/lib/strata-rmm /etc/strata-rmm
EXPOSE 8080
ENTRYPOINT ["strata-rmm"]

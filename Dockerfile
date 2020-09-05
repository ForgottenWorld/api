FROM golang:alpine AS build
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

ENV USER=appuser
ENV UID=10001

RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    "${USER}"

# Set the Current Working Directory inside the container
WORKDIR /go/app

# Copy go mod and sum files
COPY go.mod go.sum .

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download
RUN go mod verify

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -ldflags='-w -s -extldflags "-static"' -a \
      -o /go/bin/fwapi .

FROM scratch

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

COPY --from=builder /go/bin/fwapi /go/bin/fwapi

USER appuser:appuser

# Expose port 8001 to the outside world
EXPOSE 8001

# Command to run the executable
ENTRYPOINT ["/go/bin/fwapi"]

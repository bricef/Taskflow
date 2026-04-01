# Build stage
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ENV LDFLAGS="-X github.com/bricef/taskflow/internal/version.Version=${VERSION}"

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o /taskflow-server ./cmd/taskflow-server
RUN CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o /taskflow ./cmd/taskflow
RUN CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o /taskflow-mcp ./cmd/taskflow-mcp

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /taskflow-server /usr/local/bin/taskflow-server
COPY --from=builder /taskflow /usr/local/bin/taskflow
COPY --from=builder /taskflow-mcp /usr/local/bin/taskflow-mcp

RUN mkdir -p /data
WORKDIR /data

ENV TASKFLOW_DB_PATH=/data/taskflow.db
ENV TASKFLOW_LISTEN_ADDR=:8374

EXPOSE 8374

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- http://localhost:8374/health || exit 1

ENTRYPOINT ["taskflow-server"]

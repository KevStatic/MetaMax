FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /bin/comput3-server ./cmd/server

# ── Runtime ────────────────────────────────────────────────────────────────

FROM alpine:3.20

# git is required at runtime: the scanner shallow-clones repositories to
# analyze them, both for /repos/scan and for the agent's in-session analysis.
RUN apk add --no-cache ca-certificates tzdata git

# Deployed workloads get a per-container directory under /vm-storage, which the
# backend creates and then bind-mounts into the workload. A named volume is
# mounted here and shared with the dind daemon so both sides see the same path;
# creating it in the image first makes Docker seed that volume with this
# ownership, so the unprivileged server can still write to it.
RUN mkdir -p /vm-storage/containers && chown -R nobody:nobody /vm-storage

COPY --from=builder /bin/comput3-server /bin/comput3-server

EXPOSE 8080

USER nobody:nobody

ENTRYPOINT ["/bin/comput3-server"]

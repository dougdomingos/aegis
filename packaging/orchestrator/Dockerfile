# === Build stage =============================================================
# Generates the gRPC code (pkg/pb is gitignored) and compiles a static binary.
FROM golang:1.26-alpine AS build
RUN apk add --no-cache make protobuf protobuf-dev
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1

ENV PATH="/go/bin:${PATH}"

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN make proto \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/aegis-orchestrator ./cmd/orchestrator

# === Runtime stage ===========================================================
# Minimal Alpine with a non-root user owning the data directory.
FROM alpine:3.22
RUN addgroup -S aegis \
 && adduser -S -G aegis -h /var/lib/aegis aegis \
 && mkdir -p /var/lib/aegis \
 && chown aegis:aegis /var/lib/aegis

COPY --from=build /out/aegis-orchestrator /usr/local/bin/aegis-orchestrator

# HTTP REST API and gRPC propagation server.
EXPOSE 8080 9090

# Defaults; override at runtime. Secrets and the lab key must be mounted, not
# baked into the image:
#   AEGIS_LAB_KEY, AEGIS_SESSION_SECRET (required).
# The machines registry is imported at runtime via POST /machines/import.
ENV AEGIS_ADDR=:8080 \
    AEGIS_GRPC_ADDR=:9090 \
    AEGIS_DB=/var/lib/aegis/aegis.db

VOLUME ["/var/lib/aegis"]
USER aegis
WORKDIR /var/lib/aegis

ENTRYPOINT ["/usr/local/bin/aegis-orchestrator"]

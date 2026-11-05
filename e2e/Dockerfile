# Image for the Aegis end-to-end suite: both binaries in one image, selected
# per container by entrypoint override. Builder layer is cached by go.mod/go.sum.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN apk add --no-cache protobuf protobuf-dev make
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
ENV PATH="/root/go/bin:${PATH}"
COPY . .
RUN make proto
RUN CGO_ENABLED=0 go build -o /out/aegis-orchestrator ./cmd/orchestrator \
 && CGO_ENABLED=0 go build -o /out/aegis-sentinel ./cmd/sentinel

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/aegis-orchestrator /usr/local/bin/aegis-orchestrator
COPY --from=build /out/aegis-sentinel /usr/local/bin/aegis-sentinel
CMD ["/usr/local/bin/aegis-orchestrator"]

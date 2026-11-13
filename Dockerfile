ARG GO_VERSION=1.27

FROM golang:${GO_VERSION} AS builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . /workspace

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o grafana-git-sync .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /workspace/grafana-git-sync /usr/local/bin/grafana-git-sync
ENTRYPOINT ["/usr/local/bin/grafana-git-sync"]

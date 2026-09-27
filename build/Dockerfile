# syntax=docker/dockerfile:1
# Builds and tests without a local Go toolchain (from the repository root):
#   docker buildx build -f build/Dockerfile --target test --progress=plain .
#   docker buildx build -f build/Dockerfile --target fmt --output . .
ARG GO_IMAGE=golang:1.24
FROM ${GO_IMAGE} AS base
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY . .

FROM base AS formatted
RUN gofmt -w .

FROM scratch AS fmt
COPY --from=formatted /src/ /

FROM base AS test
RUN --mount=type=cache,target=/root/.cache/go-build \
    test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }; \
    go vet ./... && go test -count=1 ./...

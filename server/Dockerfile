# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/vesper ./cmd/vesper

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/vesper /usr/local/bin/vesper
ARG VERSION=dev
ENV LAZYAGENTS_VERSION=${VERSION}
EXPOSE 8000
ENTRYPOINT ["vesper"]
CMD ["serve"]

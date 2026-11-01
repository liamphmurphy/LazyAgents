# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/lazyagents ./cmd/lazyagents

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lazyagents /usr/local/bin/lazyagents
ARG VERSION=dev
ENV LAZYAGENTS_VERSION=${VERSION}
EXPOSE 8000
ENTRYPOINT ["lazyagents"]
CMD ["serve"]

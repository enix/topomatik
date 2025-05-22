FROM golang:1.24-bookworm AS build

WORKDIR /build

COPY ./go.* ./

RUN go mod download

COPY cmd cmd
COPY internal internal

ARG version=latest

RUN --mount=type=cache,target="/root/.cache/go-build" \
  go build -v -trimpath -ldflags "-X main.Version=$version" -o topomatik ./cmd/main.go

###########################################

FROM ubuntu:22.04

LABEL maintainer="Paul Laffitte <paul.laffitte@enix.fr>" \
  org.opencontainers.image.title="topomatik" \
  org.opencontainers.image.description="You don't have to do anything, it's all Topomatik™!" \
  org.opencontainers.image.url="https://github.com/enix/topomatik" \
  org.opencontainers.image.source="https://github.com/enix/topomatik/blob/main/Dockerfile" \
  org.opencontainers.image.documentation="https://github.com/enix/topomatik/blob/main/README.md" \
  org.opencontainers.image.authors="Paul Laffitte <paul.laffitte@enix.fr>" \
  org.opencontainers.image.licenses="MIT"

COPY --from=build /build/topomatik /usr/local/bin/

CMD [ "topomatik" ]


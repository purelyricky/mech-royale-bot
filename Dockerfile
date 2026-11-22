FROM golang:1.26.1-bookworm@sha256:ab3d6955bbc813a0f3fdf220c1d817dd89c0b3f283777db8ece4a32fe7858edd AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOTOOLCHAIN=local
RUN go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/mechbot ./cmd/mechbot

FROM scratch
COPY --from=build /out/mechbot /mechbot
USER 65534:65534
ENTRYPOINT ["/mechbot"]

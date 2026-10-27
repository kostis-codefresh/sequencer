FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY ui ui
COPY cmd/sequencer-web cmd/sequencer-web
# .git is not in the build context, so the version comes in as a build arg.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.version=${VERSION}" -o /out/sequencer-web ./cmd/sequencer-web

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/sequencer-web /sequencer-web

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/sequencer-web"]

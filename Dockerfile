FROM golang:1.26-alpine3.24 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY ui ui
COPY cmd/sequencer-web cmd/sequencer-web
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/sequencer-web ./cmd/sequencer-web

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/sequencer-web /sequencer-web

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/sequencer-web"]

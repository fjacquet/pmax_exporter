# syntax=docker/dockerfile:1
FROM golang:1.26.4 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/pmax_exporter .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/pmax_exporter /pmax_exporter
USER nonroot:nonroot
EXPOSE 9104
ENTRYPOINT ["/pmax_exporter"]
CMD ["--config", "/etc/pmax_exporter/config.yaml"]

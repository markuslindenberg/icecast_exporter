FROM golang:alpine AS build

WORKDIR /go/src/icecast_exporter

RUN apk add --no-cache git

COPY . /go/src/icecast_exporter

RUN go build icecast_exporter.go

# Final stage
FROM alpine

COPY --from=build /go/bin/icecast_exporter /icecast_exporter

EXPOSE 9146
USER nobody
ENTRYPOINT ["/icecast_exporter"]

FROM golang:alpine

RUN mkdir -p /go/src/icecast_exporter
WORKDIR /go/src/icecast_exporter

COPY . /go/src/icecast_exporter

RUN apk add --no-cache --virtual .git git ; go-wrapper download ; apk del .git
RUN go-wrapper install

EXPOSE 9146
USER nobody
ENTRYPOINT ["icecast_exporter"]

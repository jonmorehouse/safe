FROM golang:latest

RUN go get gopkg.in/yaml.v2

ADD . /src
RUN mkdir -p /go/src/github.com/jonmorehouse && \
	ln -s /src /go/src/github.com/jonmorehouse/safe && \
	mkdir /output && \
	cd /src/bin && \
	CGO_ENABLED=0 GOOS=linux go build -o /output/safe .

FROM alpine:latest
COPY --from=0 /output/safe /bin
ENTRYPOINT ["/bin/safe"]

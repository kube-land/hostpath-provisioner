ARG IMAGE=alpine:3.9.3

FROM golang:1.12.3-alpine as builder
WORKDIR ${GOPATH}/src/github.com/appwavelets/hostpath-provisioner
COPY . ./
RUN apk add --update gcc libc-dev linux-headers
RUN CGO_ENABLED=1 GOOS=linux go build -o /usr/bin/hostpath-provisioner

FROM ${IMAGE}
COPY --from=builder /usr/bin/hostpath-provisioner /usr/bin/
ENTRYPOINT ["/usr/bin/hostpath-provisioner"]

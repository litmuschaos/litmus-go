# Multi-stage docker build
# Build stage
FROM golang:1.18 AS builder

ARG TARGETOS=linux
ARG TARGETARCH

ADD . /litmus-go
WORKDIR /litmus-go

RUN export GOOS=${TARGETOS} && \
    export GOARCH=${TARGETARCH}

RUN CGO_ENABLED=0 go build -o /output/experiments ./bin/experiment
RUN CGO_ENABLED=0 go build -o /output/helpers ./bin/helper

FROM alpine:3.15.0 AS dep

# Install generally useful things
RUN apk --update add \
        sudo \
        iproute2 \
        iptables


# Packaging stage
# Image source: https://github.com/litmuschaos/test-tools/blob/master/custom/hardened-alpine/experiment/Dockerfile
# The base image is non-root (have litmus user) with default litmus directory.
FROM litmuschaos/experiment-alpine

LABEL maintainer="LitmusChaos"

COPY --from=builder /output/ /litmus
COPY --from=dep /usr/bin/sudo /usr/bin/sudo
COPY --from=dep /usr/lib/sudo /usr/lib/sudo
COPY --from=dep /sbin/tc /sbin/
COPY --from=dep /sbin/iptables /sbin/

#Copying Necessary Files
COPY ./pkg/cloud/aws/common/ssm-docs/LitmusChaos-AWS-SSM-Docs.yml .

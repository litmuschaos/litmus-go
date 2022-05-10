# Multi-stage docker build
# Build stage
FROM golang:1.17 AS builder

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

RUN wget https://github.com/Shopify/toxiproxy/releases/download/v2.4.0/toxiproxy-cli-linux-amd64 && \
    chmod +x /toxiproxy-cli-linux-amd64 && \
    wget https://github.com/Shopify/toxiproxy/releases/download/v2.4.0/toxiproxy-server-linux-amd64 && \
    chmod +x /toxiproxy-server-linux-amd64

# Packaging stage
# Image source: https://github.com/litmuschaos/test-tools/blob/master/custom/hardened-alpine/experiment/Dockerfile
# The base image is non-root (have litmus user) with default litmus directory.
FROM litmuschaos/experiment-alpine

LABEL maintainer="LitmusChaos"

COPY --from=builder /output/ /litmus
COPY --from=dep /usr/bin/sudo /usr/bin/
COPY --from=dep /sbin/tc /sbin/
COPY --from=dep /sbin/iptables /sbin/

COPY --from=dep /toxiproxy-server-linux-* /litmus/toxiproxy
COPY --from=dep /toxiproxy-cli-linux-* /litmus/toxiproxy-cli

RUN ls 

#Copying Necessary Files
COPY ./pkg/cloud/aws/common/ssm-docs/LitmusChaos-AWS-SSM-Docs.yml .

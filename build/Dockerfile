# Multi-stage docker build
# Build stage
FROM golang:alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH

ADD . /litmus-go
WORKDIR /litmus-go

RUN export GOOS=${TARGETOS} && \
    export GOARCH=${TARGETARCH}

RUN CGO_ENABLED=0 go build -o /output/experiments ./bin/experiment
RUN CGO_ENABLED=0 go build -o /output/helpers ./bin/helper

FROM alpine:3.14.2 AS dep

# Install generally useful things
RUN apk --update add \
        sudo \
        iproute2

# Packaging stage
# Image source: https://github.com/litmuschaos/test-tools/blob/master/custom/hardened-alpine/experiment/Dockerfile
# The base image is non-root (have litmus user) with default litmus directory.
FROM litmuschaos/experiment-alpine

LABEL maintainer="LitmusChaos"

COPY --from=builder /output/ /litmus
COPY --from=dep /usr/bin/sudo /usr/bin/
COPY --from=dep /sbin/tc /sbin/

#Copying Necessary Files
COPY ./pkg/cloud/aws/common/ssm-docs/LitmusChaos-AWS-SSM-Docs.yml ./litmus/LitmusChaos-AWS-SSM-Docs.yml

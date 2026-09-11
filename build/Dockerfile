# GLOBAL ARGUMENT: Base Image
ARG BASE_IMAGE=registry.access.redhat.com/ubi9/ubi:9.8

# Multi-stage docker build
# Build stage
FROM golang:1.27.2 AS builder

ARG TARGETOS=linux
ARG TARGETARCH

ADD . /litmus-go
WORKDIR /litmus-go

RUN export GOOS=${TARGETOS} && \
    export GOARCH=${TARGETARCH}

RUN CGO_ENABLED=0 go build -o /output/experiments ./bin/experiment
RUN CGO_ENABLED=0 go build -o /output/helpers ./bin/helper

# Compile stale tools securely from source (Bypassing CVEs)
RUN CGO_ENABLED=0 go install github.com/nalbury/promql-cli@latest && \
    CGO_ENABLED=0 go install github.com/Shopify/toxiproxy/v2/cmd/toxiproxy-cli@latest && \
    CGO_ENABLED=0 go install github.com/Shopify/toxiproxy/v2/cmd/toxiproxy-server@latest

# Packaging stage
FROM ${BASE_IMAGE}

LABEL maintainer="LitmusChaos"

ARG TARGETARCH
ARG LITMUS_VERSION

# Install generally useful things
RUN yum install -y \
    sudo \
    sshpass \
    procps-ng \
    openssh-clients 
    
# EPEL repo for stress-ng
RUN yum install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-$(rpm -E %rhel).noarch.rpm

# tc binary & iptables
RUN yum install -y iproute iptables iptables-services

# stress-ng
RUN if [ $(rpm -E %rhel) -eq 9 ]; then \
      yum install -y https://yum.oracle.com/repo/OracleLinux/OL9/appstream/$(uname -m)/getPackage/Judy-1.0.5-28.el9.$(uname -m).rpm && \
      yum install -y https://yum.oracle.com/repo/OracleLinux/OL9/appstream/$(uname -m)/getPackage/stress-ng-0.14.00-2.el9.$(uname -m).rpm; \
    else \
      yum install -y stress-ng; \
    fi && yum clean all

# Installing Kubectl (Updated to modern registry with fail-fast flag)
ENV KUBE_LATEST_VERSION="v1.37.0"
RUN curl -fL https://dl.k8s.io/release/${KUBE_LATEST_VERSION}/bin/linux/${TARGETARCH}/kubectl -o /usr/bin/kubectl && \
    chmod 755 /usr/bin/kubectl

# Installing crictl binaries
RUN curl -fL https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.37.0/crictl-v1.37.0-linux-${TARGETARCH}.tar.gz --output crictl-v1.37.0-linux-${TARGETARCH}.tar.gz && \
    tar zxvf crictl-v1.37.0-linux-${TARGETARCH}.tar.gz -C /sbin && \
    rm -f crictl-v1.37.0-linux-${TARGETARCH}.tar.gz && \
    chmod 755 /sbin/crictl

# Installing pause cli binaries
RUN curl -fL https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/pause-linux-${TARGETARCH} --output /usr/bin/pause && chmod 755 /usr/bin/pause

# Installing dns_interceptor cli binaries
RUN curl -fL https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/dns_interceptor --output /sbin/dns_interceptor && chmod 755 /sbin/dns_interceptor

# Installing nsutil cli binaries and shared lib
RUN curl -fL https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/nsutil-linux-${TARGETARCH} --output /sbin/nsutil && chmod 755 /sbin/nsutil
RUN curl -fL https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/nsutil_${TARGETARCH}.so --output /usr/local/lib/nsutil.so && chmod 755 /usr/local/lib/nsutil.so

ENV APP_USER=litmus
ENV APP_DIR="/$APP_USER"
ENV DATA_DIR="$APP_DIR/data"

# The USERD_ID of user (Updated to /bin/bash for PAM compatibility)
ENV APP_USER_ID=2000
RUN useradd -s /bin/bash -u $APP_USER_ID -m -d $APP_DIR $APP_USER

# change to 0(root) group because openshift will run container with arbitrary uid as a member of root group
RUN chgrp -R 0 "$APP_DIR" && chmod -R g=u "$APP_DIR"

# Giving sudo to all users (required for almost all experiments)
RUN echo 'ALL ALL=(ALL:ALL) NOPASSWD: ALL' >> /etc/sudoers

WORKDIR $APP_DIR

# Copy internal Litmus code
COPY --from=builder /output/ .

# Copy secure third-party tools from builder stage
COPY --from=builder /go/bin/promql-cli /usr/bin/promql
COPY --from=builder /go/bin/toxiproxy-cli /sbin/toxiproxy-cli
COPY --from=builder /go/bin/toxiproxy-server /sbin/toxiproxy-server
RUN chmod 755 /usr/bin/promql /sbin/toxiproxy-cli /sbin/toxiproxy-server

COPY --from=docker:27.0.3 /usr/local/bin/docker /sbin/docker
RUN chmod 755 /sbin/docker

# Set permissions and ownership for the copied binaries
RUN chmod 755 ./experiments ./helpers && \
    chown ${APP_USER}:0 ./experiments ./helpers

# Surgical ownership (Drops the wildcard to protect PAM/sudo)
RUN chown ${APP_USER}:0 /usr/bin/promql /sbin/toxiproxy-cli /sbin/toxiproxy-server /usr/bin/kubectl /sbin/crictl /sbin/docker /usr/bin/pause /sbin/dns_interceptor /sbin/nsutil && \
    chown root:root /usr/bin/sudo && \
    chmod 4755 /usr/bin/sudo

# Copying Necessary Files
COPY ./pkg/cloud/aws/common/ssm-docs/LitmusChaos-AWS-SSM-Docs.yml ./LitmusChaos-AWS-SSM-Docs.yml
RUN chown ${APP_USER}:0 ./LitmusChaos-AWS-SSM-Docs.yml && chmod 755 ./LitmusChaos-AWS-SSM-Docs.yml

USER ${APP_USER}

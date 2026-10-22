# GLOBAL ARGUMENT: Base Image
ARG BASE_IMAGE=registry.access.redhat.com/ubi9/ubi:9.4

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

# Compile stale tools securely by force-patching their dependencies before build
RUN git clone https://github.com/nalbury/promql-cli.git /tmp/promql-cli && \
    cd /tmp/promql-cli && go get golang.org/x/text@latest golang.org/x/oauth2@latest && go mod tidy && \
    go build -o /go/bin/promql-cli .

RUN git clone --branch v2.1.4 https://github.com/Shopify/toxiproxy.git /tmp/toxiproxy && \
    cd /tmp/toxiproxy && go get golang.org/x/text@latest golang.org/x/oauth2@latest && go mod tidy && \
    go build -o /go/bin/toxiproxy-server ./cmd/toxiproxy-server && \
    go build -o /go/bin/toxiproxy-cli ./cmd/toxiproxy-cli
# Packaging stage
FROM ${BASE_IMAGE}

LABEL maintainer="LitmusChaos"

ARG TARGETARCH
ARG LITMUS_VERSION

# Force OS security patches
RUN yum upgrade -y && yum clean all

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

# Installing Kubectl
ENV KUBE_LATEST_VERSION="v1.31.0"
RUN curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/linux/${TARGETARCH}/kubectl -o /usr/bin/kubectl && \
    chmod 755 /usr/bin/kubectl

# Installing crictl binaries
RUN curl -L https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.31.1/crictl-v1.31.1-linux-${TARGETARCH}.tar.gz --output crictl.tar.gz && \
    tar zxvf crictl.tar.gz -C /sbin && rm -f crictl.tar.gz && \
    chmod 755 /sbin/crictl

# Installing Litmus test tools
RUN curl -L https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/pause-linux-${TARGETARCH} --output /usr/bin/pause && chmod 755 /usr/bin/pause
RUN curl -L https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/dns_interceptor --output /sbin/dns_interceptor && chmod 755 /sbin/dns_interceptor
RUN curl -L https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/nsutil-linux-${TARGETARCH} --output /sbin/nsutil && chmod 755 /sbin/nsutil
RUN curl -L https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/nsutil_${TARGETARCH}.so --output /usr/local/lib/nsutil.so && chmod 755 /usr/local/lib/nsutil.so

ENV APP_USER=litmus
ENV APP_DIR="/$APP_USER"
ENV DATA_DIR="$APP_DIR/data"

ENV APP_USER_ID=2000
# FIX: Use bash instead of true to support PAM/sudo authentication in E2E tests
RUN useradd -s /bin/bash -u $APP_USER_ID -m -d $APP_DIR $APP_USER

# change to 0(root) group
RUN chgrp -R 0 "$APP_DIR" && chmod -R g=u "$APP_DIR"
RUN echo 'ALL ALL=(ALL:ALL) NOPASSWD: ALL' >> /etc/sudoers

WORKDIR $APP_DIR

# Copy Litmus binaries
COPY --from=builder /output/ .

# Copy Securely Compiled Tools
COPY --from=builder /go/bin/promql-cli /usr/bin/promql
COPY --from=builder /go/bin/toxiproxy-cli /sbin/toxiproxy-cli
COPY --from=builder /go/bin/toxiproxy-server /sbin/toxiproxy-server
RUN chmod 755 /usr/bin/promql /sbin/toxiproxy-cli /sbin/toxiproxy-server

COPY --from=docker:27.0.3 /usr/local/bin/docker /sbin/docker
RUN chmod 755 /sbin/docker

# Set permissions and ownership for the copied binaries
RUN chmod 755 ./experiments ./helpers && \
    chown ${APP_USER}:0 ./experiments ./helpers

# FIX: Targeted chown instead of wildcard to preserve system sudo/PAM files
RUN chown ${APP_USER}:0 /usr/bin/promql /sbin/toxiproxy-cli /sbin/toxiproxy-server /usr/bin/kubectl /sbin/crictl /usr/bin/pause /sbin/dns_interceptor /sbin/nsutil /sbin/docker && \
    chown root:root /usr/bin/sudo && \
    chmod 4755 /usr/bin/sudo

# Copying Necessary Files
COPY ./pkg/cloud/aws/common/ssm-docs/LitmusChaos-AWS-SSM-Docs.yml ./LitmusChaos-AWS-SSM-Docs.yml
RUN chown ${APP_USER}:0 ./LitmusChaos-AWS-SSM-Docs.yml && chmod 755 ./LitmusChaos-AWS-SSM-Docs.yml

USER ${APP_USER}

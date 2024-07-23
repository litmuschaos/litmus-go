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

# Packaging stage
FROM registry.access.redhat.com/ubi9/ubi:9.4

LABEL maintainer="LitmusChaos"

ARG TARGETARCH
ARG LITMUS_VERSION

# Install generally useful things
RUN yum install -y \
    sudo \
    sshpass \
    procps
    
# tc binary
RUN yum install -y https://dl.rockylinux.org/pub/rocky/9/devel/$(uname -m)/os/Packages/i/iproute-6.2.0-6.el9_4.$(uname -m).rpm
RUN yum install -y https://dl.rockylinux.org/pub/rocky/9/devel/$(uname -m)/os/Packages/i/iproute-tc-6.2.0-6.el9_4.$(uname -m).rpm && \
    chmod 755 /sbin/tc
    
# iptables
RUN yum install -y https://dl.rockylinux.org/pub/rocky/9/devel/$(uname -m)/os/Packages/i/iptables-libs-1.8.10-2.el9.$(uname -m).rpm
RUN yum install -y https://dl.fedoraproject.org/pub/epel/9/Everything/$(uname -m)/Packages/i/iptables-legacy-libs-1.8.10-2.2.el9.$(uname -m).rpm
RUN yum install -y https://dl.fedoraproject.org/pub/epel/9/Everything/$(uname -m)/Packages/i/iptables-legacy-1.8.10-2.2.el9.$(uname -m).rpm && \
    mv /usr/sbin/iptables /usr/local/bin/iptables && \
    chmod 755 /usr/local/bin/iptables

# stress-ng 
RUN yum install -y https://yum.oracle.com/repo/OracleLinux/OL9/appstream/$(uname -m)/getPackage/Judy-1.0.5-28.el9.$(uname -m).rpm
RUN yum install -y https://yum.oracle.com/repo/OracleLinux/OL9/appstream/$(uname -m)/getPackage/stress-ng-0.14.00-2.el9.$(uname -m).rpm && \
    mv /usr/bin/stress-ng /sbin/stress-ng && \
    chmod 755 /sbin/stress-ng

#Installing Kubectl
ENV KUBE_LATEST_VERSION="v1.30.3"
RUN curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/linux/${TARGETARCH}/kubectl -o     /usr/local/bin/kubectl && \
    chmod 755 /usr/local/bin/kubectl

#Installing crictl binaries
RUN curl -L https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.30.1/crictl-v1.30.1-linux-${TARGETARCH}.tar.gz --output crictl-v1.30.1-linux-${TARGETARCH}.tar.gz && \
    tar zxvf crictl-v1.30.1-linux-${TARGETARCH}.tar.gz -C /sbin && \
    chmod 755 /sbin/crictl

#Installing promql cli binaries
RUN curl -L https://github.com/chaosnative/promql-cli/releases/download/3.0.0-beta6/promql_linux_${TARGETARCH} --output /usr/local/bin/promql && chmod 755 /usr/local/bin/promql

#Installing pause cli binaries
RUN curl -L https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/pause-linux-${TARGETARCH} --output /usr/local/bin/pause && chmod 755 /usr/local/bin/pause

#Installing dns_interceptor cli binaries
RUN curl -L https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/dns_interceptor --output /sbin/dns_interceptor && chmod 755 /sbin/dns_interceptor

#Installing nsutil cli binaries
RUN curl -L https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/nsutil-linux-${TARGETARCH} --output /sbin/nsutil && chmod 755 /sbin/nsutil

#Installing nsutil shared lib
RUN curl -L https://github.com/litmuschaos/test-tools/releases/download/${LITMUS_VERSION}/nsutil_${TARGETARCH}.so --output /sbin/nsutil.so && chmod 755 /sbin/nsutil.so

# Installing toxiproxy binaries
RUN curl -L https://litmus-http-proxy.s3.amazonaws.com/cli/toxiproxy-cli-linux-${TARGETARCH}.tar.gz --output toxiproxy-cli-linux-${TARGETARCH}.tar.gz && \
    tar zxvf toxiproxy-cli-linux-${TARGETARCH}.tar.gz -C /sbin/ && \
    chmod 755 /sbin/toxiproxy-cli
RUN curl -L https://litmus-http-proxy.s3.amazonaws.com/server/toxiproxy-server-linux-${TARGETARCH}.tar.gz --output toxiproxy-server-linux-${TARGETARCH}.tar.gz && \
    tar zxvf toxiproxy-server-linux-${TARGETARCH}.tar.gz -C /sbin/ && \
    chmod 755 /sbin/toxiproxy-server

ENV APP_USER=litmus
ENV APP_DIR="/$APP_USER"
ENV DATA_DIR "$APP_DIR/data"
ENV CONF_DIR "$APP_DIR/conf"

# Add custom user and setup home directory
RUN useradd -ms /bin/bash -u 2000 $APP_USER && \
    mkdir -p "$DATA_DIR" "$CONF_DIR" && \
    chown -R "$APP_USER:0" "$DATA_DIR" "$CONF_DIR" && \
    echo "$APP_USER ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/$APP_USER && \
    chmod 0440 /etc/sudoers.d/$APP_USER

COPY --from=builder /output/ $APP_DIR/

COPY --from=docker:27.0.3 /usr/local/bin/docker /sbin/docker
RUN chmod 755 /sbin/docker 

# Set permissions and ownership for the copied binaries
RUN chmod 755 $APP_DIR/experiments $APP_DIR/helpers && \
    chown ${APP_USER}:0 $APP_DIR/experiments $APP_DIR/helpers

# Set ownership for binaries in /sbin and /usr/local/bin
RUN chown ${APP_USER}:0 /sbin/* /usr/local/bin/*

# Copying Necessary Files
COPY ./pkg/cloud/aws/common/ssm-docs/LitmusChaos-AWS-SSM-Docs.yml $APP_DIR/LitmusChaos-AWS-SSM-Docs.yml
RUN chown ${APP_USER}:0 $APP_DIR/LitmusChaos-AWS-SSM-Docs.yml && chmod 755 $APP_DIR/LitmusChaos-AWS-SSM-Docs.yml

WORKDIR $APP_DIR
USER ${APP_USER}
FROM ubuntu:bionic

LABEL maintainer="LitmusChaos"

ARG TARGETARCH

#Installing necessary ubuntu packages
RUN apt-get update && apt-get install -y curl bash systemd iproute2 stress-ng openssh-client

#Installing Kubectl
ENV KUBE_LATEST_VERSION="v1.18.0"
RUN curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/linux/${TARGETARCH}/kubectl -o     /usr/local/bin/kubectl && \
    chmod +x /usr/local/bin/kubectl

#Installing crictl binaries
RUN curl -L https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.16.0/crictl-v1.16.0-linux-${TARGETARCH}.tar.gz --output crictl-v1.16.0-linux-${TARGETARCH}.tar.gz && \
    tar zxvf crictl-v1.16.0-linux-${TARGETARCH}.tar.gz -C /usr/local/bin
    
#Installing pumba binaries
ENV PUMBA_VERSION="0.6.5"
RUN curl -L https://github.com/alexei-led/pumba/releases/download/${PUMBA_VERSION}/pumba_linux_${TARGETARCH} --output /usr/local/bin/pumba && chmod +x /usr/local/bin/pumba

#Installing promql cli binaries
RUN curl -L https://github.com/litmuschaos/test-tools/raw/master/custom/promql-cli/promql-linux-${TARGETARCH} --output /usr/local/bin/promql && chmod +x /usr/local/bin/promql

RUN if [[${TARGETARCH} == "arm64"]]; then \
    curl -L https://get.docker.com/builds/Linux/aarch64/docker-1.13.1.tgz --output docker-cli && tar -zxvf docker-cli docker/docker && mv docker/docker /usr/local/bin/docker && chmod +x /usr/local/bin/docker; \
  else \
    curl -L https://get.docker.com/builds/Linux/x86_64/docker-1.13.1.tgz --output docker-cli && tar -zxvf docker-cli docker/docker && mv docker/docker /usr/local/bin/docker && chmod +x /usr/local/bin/docker; \
  fi
  
#Copying Necessary Files
COPY ./build/_output/${TARGETARCH} ./litmus

WORKDIR /litmus

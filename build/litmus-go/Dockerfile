FROM ubuntu:18.04 

LABEL maintainer="LitmusChaos"

ARG TARGETARCH

# intall gcc and supporting packages
RUN apt-get update && apt-get install -yq make gcc && \
    apt-get install -y curl

WORKDIR /code

ENV USER_UID=1001 \
    USER_NAME=litmus-go

#Install stress-ng
RUN curl -L https://github.com/uditgaurav/test-tools/releases/download/v0.1.0/stress-ng_amd64 -o /usr/local/bin/stress-ng && \
    chmod +x /usr/local/bin/stress-ng

RUN file="$(ls -a /usr/local/bin/)" && echo $file

#Installing Kubectl
ENV KUBE_LATEST_VERSION="v1.18.0"
RUN curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/linux/${TARGETARCH}/kubectl -o     /usr/local/bin/kubectl && \
    chmod +x /usr/local/bin/kubectl

#Installing crictl binaries
RUN curl -L https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.16.0/crictl-v1.16.0-linux-amd64.tar.gz --output crictl-v1.16.0-linux-amd64.tar.gz && \
    tar zxvf crictl-v1.16.0-linux-amd64.tar.gz -C /usr/local/bin
    
#Installing pumba binaries
ENV PUMBA_VERSION="0.6.5"
RUN curl -L https://github.com/alexei-led/pumba/releases/download/${PUMBA_VERSION}/pumba_linux_${TARGETARCH} --output /usr/local/bin/pumba && chmod +x /usr/local/bin/pumba

#Copying Necessary Files
COPY ./build/_output/${TARGETARCH} ./experiments

USER ${USER_UID}

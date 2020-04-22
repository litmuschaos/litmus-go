FROM ubuntu:bionic

LABEL maintainer="LitmusChaos"

#Installing necessary ubuntu packages
RUN apt-get clean && \
    apt-get update --fix-missing || true && \
    apt-get install -y --no-install-recommends python-minimal python-pip netcat iproute2 jq sshpass \
    curl openssh-client python-setuptools && rm -rf /var/lib/apt/lists/* 

RUN pip install --upgrade pip

RUN touch /mnt/parameters.yml

#Installing gcloud 
RUN curl -sSL https://sdk.cloud.google.com | bash
ENV PATH $PATH:/root/google-cloud-sdk/bin
RUN gcloud --version

#Installing Kubectl
ENV KUBE_LATEST_VERSION="v1.12.0"
RUN curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/linux/amd64/kubectl -o /usr/local/bin/kubectl && \
    chmod +x /usr/local/bin/kubectl

#Installing go
RUN curl -s https://storage.googleapis.com/golang/go1.2.2.linux-amd64.tar.gz| tar -v -C /usr/local -xz
# Set environment variables.
ENV PATH $PATH:/usr/local/go/bin

#Copying Necessary Files
COPY ./chaoslib ./chaoslib/
COPY ./pkg ./pkg/
COPY ./experiments ./experiments
COPY ./build/_output ./build/_output
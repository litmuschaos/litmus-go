FROM alpine:latest

LABEL maintainer="LitmusChaos"

#Installing necessary ubuntu packages
RUN apk update && apk add curl bash
RUN apk add --no-cache libc6-compat

#Installing Kubectl
ENV KUBE_LATEST_VERSION="v1.18.0"
RUN curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/linux/amd64/kubectl -o /usr/local/bin/kubectl && \
    chmod +x /usr/local/bin/kubectl

#Copying Necessary Files
COPY ./build/_output ./experiments

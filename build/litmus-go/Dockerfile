FROM ubuntu:18.04 as builder

# intall gcc and supporting packages
RUN apt-get update && apt-get install -yq make gcc

WORKDIR /code

# download stress-ng sources
ARG STRESS_NG_VERSION
ENV STRESS_NG_VERSION ${STRESS_NG_VERSION:-0.10.10}
ADD https://github.com/ColinIanKing/stress-ng/archive/V${STRESS_NG_VERSION}.tar.gz .
RUN tar -xf V${STRESS_NG_VERSION}.tar.gz && mv stress-ng-${STRESS_NG_VERSION} stress-ng

#install stress
RUN apt-get install stress

# make static version
WORKDIR /code/stress-ng
RUN STATIC=1 make

FROM alpine:latest

LABEL maintainer="LitmusChaos"

#Installing necessary ubuntu packages
RUN apk update && apk add curl bash
RUN apk add --no-cache libc6-compat

#Installing Kubectl
ENV KUBE_LATEST_VERSION="v1.18.0"
RUN curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/linux/amd64/kubectl -o /usr/local/bin/kubectl && \
    chmod +x /usr/local/bin/kubectl

#Installing crictl binaries
RUN curl -L https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.16.0/crictl-v1.16.0-linux-amd64.tar.gz --output crictl-v1.16.0-linux-amd64.tar.gz && \
    tar zxvf crictl-v1.16.0-linux-amd64.tar.gz -C /usr/local/bin

#Installing pumba binaries
ENV PUMBA_VERSION="0.6.5"
RUN curl -L https://github.com/alexei-led/pumba/releases/download/${PUMBA_VERSION}/pumba_linux_amd64 --output /usr/local/bin/pumba && chmod +x /usr/local/bin/pumba

COPY --from=builder /code/stress-ng/stress-ng /

#Copying Necessary Files
COPY ./build/_output ./experiments

FROM ubuntu:18.04 as builder

# intall gcc and supporting packages
RUN apt-get update && apt-get install -yq make gcc

WORKDIR /code

# download stress-ng sources
ARG STRESS_NG_VERSION
ENV STRESS_NG_VERSION ${STRESS_NG_VERSION:-0.10.10}
ADD https://github.com/ColinIanKing/stress-ng/archive/V${STRESS_NG_VERSION}.tar.gz .
RUN tar -xf V${STRESS_NG_VERSION}.tar.gz && mv stress-ng-${STRESS_NG_VERSION} stress-ng

# make static version
WORKDIR /code/stress-ng
RUN STATIC=1 make

FROM ubuntu:bionic

LABEL maintainer="LitmusChaos"

ENV USER_UID=1001 \
    USER_NAME=litmus-go

#Installing necessary ubuntu packages
RUN apt-get update && apt-get install -y curl bash systemd iproute2

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
COPY ./build/_output ./litmus/experiments

RUN useradd -ms /bin/bash ${USER_NAME} 
WORKDIR /litmus
RUN chown -R ${USER_NAME}:root /litmus && \
    chown -R ${USER_NAME}:root /var && \
    chown -R ${USER_NAME}:root /run
    
USER ${USER_NAME}
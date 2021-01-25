FROM alpine:3.12

LABEL maintainer="LitmusChaos"

ARG TARGETARCH
ARG USER=litmus

RUN rm -rf /var/lib/apt/lists/*

# Install generally useful things
RUN apk --update add \
        sudo \
	htop\
        bash\
        make\
        git \
        curl\
        iproute2\
        stress-ng\
        openssh-client\
        libc6-compat \
        sshpass

# Change default shell from ash to bash
RUN sed -i -e "s/bin\/ash/bin\/bash/" /etc/passwd     

#Installing Kubectl
ENV KUBE_LATEST_VERSION="v1.18.0"
RUN curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBE_LATEST_VERSION}/bin/linux/${TARGETARCH}/kubectl -o     /usr/local/bin/kubectl && \
    chmod +x /usr/local/bin/kubectl

#Installing crictl binaries
RUN curl -L https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.16.0/crictl-v1.16.0-linux-${TARGETARCH}.tar.gz --output crictl-v1.16.0-linux-${TARGETARCH}.tar.gz && \
    tar zxvf crictl-v1.16.0-linux-${TARGETARCH}.tar.gz -C /usr/local/bin
    
#Installing pumba binaries
ENV PUMBA_VERSION="0.7.7"
RUN curl -L https://github.com/alexei-led/pumba/releases/download/${PUMBA_VERSION}/pumba_linux_${TARGETARCH} --output /usr/local/bin/pumba && chmod +x /usr/local/bin/pumba

#Installing promql cli binaries
RUN curl -L https://github.com/litmuschaos/test-tools/raw/master/custom/promql-cli/promql-linux-${TARGETARCH} --output /usr/local/bin/promql && chmod +x /usr/local/bin/promql

COPY --from=docker:19.03 /usr/local/bin/docker /usr/local/bin/

#Copying Necessary Files
COPY ./build/_output/${TARGETARCH} ./litmus

#add new user
RUN adduser -D -S $USER \
        && echo "$USER ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/$USER \
        && chmod 0440 /etc/sudoers.d/$USER

USER $USER

WORKDIR /litmus

FROM alpine:3.13

LABEL maintainer="LitmusChaos"

ARG TARGETARCH
ARG USER=litmus

ENV GLIBC_REPO=https://github.com/sgerrand/alpine-pkg-glibc
ENV GLIBC_VERSION=2.30-r0

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
#        libc6-compat \
        sshpass

RUN set -ex && \
     apk --update add libstdc++ curl ca-certificates && \
     for pkg in glibc-${GLIBC_VERSION} glibc-bin-${GLIBC_VERSION}; \
         do curl -sSL ${GLIBC_REPO}/releases/download/${GLIBC_VERSION}/${pkg}.apk -o /tmp/${pkg}.apk; done && \
     apk add --allow-untrusted /tmp/*.apk && \
     rm -v /tmp/*.apk && \
     /usr/glibc-compat/sbin/ldconfig /lib /usr/glibc-compat/lib

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

#Installing nsutil cli binaries
RUN curl -L https://github.com/litmuschaos/test-tools/raw/master/custom/nsutil/nsutil --output /usr/local/bin/nsutil && chmod +x /usr/local/bin/nsutil

#Installing dns_interceptor cli binaries
RUN curl -L https://github.com/litmuschaos/test-tools/raw/master/custom/dns_interceptor/dns_interceptor --output /usr/local/bin/dns_interceptor && chmod +x /usr/local/bin/dns_interceptor

COPY --from=docker:19.03 /usr/local/bin/docker /usr/local/bin/

#Copying Necessary Files
COPY ./build/_output/${TARGETARCH} ./litmus

#add new user
RUN adduser -D -S $USER \
        && echo "$USER ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/$USER \
        && chmod 0440 /etc/sudoers.d/$USER

USER $USER

WORKDIR /litmus

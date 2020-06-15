FROM alpine:latest

LABEL maintainer="LitmusChaos"

#Installing necessary ubuntu packages
RUN apk update && apk add curl bash
RUN apk add --no-cache libc6-compat

#Copying Necessary Files
COPY ./build/_output ./experiments

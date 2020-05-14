FROM ubuntu:bionic

LABEL maintainer="LitmusChaos"

#Installing necessary ubuntu packages
RUN apt-get update && apt-get install -y curl

#Copying Necessary Files
COPY ./build/_output ./experiments
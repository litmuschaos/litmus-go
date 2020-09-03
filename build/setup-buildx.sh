#!/bin/bash

#  set -e
#  # Configure environment so changes are picked up when the Docker daemon is restarted after upgrading
#  echo '{"experimental":true}' | sudo tee /etc/docker/daemon.json
#  export DOCKER_CLI_EXPERIMENTAL=enabled
#  sudo docker run --rm --privileged docker/binfmt:a7996909642ee92942dcd6cff44b9b95f08dad64
#   # Upgrade to Docker CE 19.03 for BuildKit support
#  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
#  sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
#  sudo apt-get update
#  sudo apt-get -y -o Dpkg::Options::="--force-confnew" install docker-ce=5:19.03.8~3-0~ubuntu-bionic # pin version for reproducibility
#  # Show info to simplify debugging and create a builder
#  sudo docker info
#  sudo docker buildx create --name builder --use
#  sudo docker buildx ls
#  sudo docker buildx build --file build/litmus-go/Dockerfile --progress plane --platform linux/arm64,linux/amd64 --tag litmuschaos/go-runner:ci .






# set -e
# sudo docker version

# supported_version=19
# docker_version=$(sudo docker version | grep Version | head -n1 | awk '{print $2}'|  cut -d'.' -f1)

# if [ "$docker_version" -lt "$supported_version" ]; then

#     # update docker to 19.03 version as buildx supports with docker 19.03+ version
#     echo "Updating docker version ..."
#     curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
#     sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
#     sudo apt-get update && sudo apt-get install -y build-essential
#     sudo apt-cache policy docker-ce
#     sudo apt-get install -y docker-ce
#     sudo docker version

# fi

# sudo apt-get install qemu-user-static -y
# sudo docker run --rm --privileged multiarch/qemu-user-static --reset -p yes i

# #set up docker buildx 
# export DOCKER_CLI_EXPERIMENTAL=enabled
# sudo apt-get update && sudo apt-get install -y build-essential
# git clone git://github.com/docker/buildx && cd buildx
# sudo make install || true
# cd .. 
# sudo rm -rf buildx
# # export DOCKER_BUILDKIT=1
# # sudo docker build --platform=local -o . git://github.com/docker/buildx
# # sudo mkdir -p ~/.docker/cli-plugins
# # sudo mv buildx ~/.docker/cli-plugins/docker-buildx
# # sudo docker buildx version

# # create a builder instance
# sudo docker buildx create --name multibuilder
# sudo docker buildx use multibuilder
# sudo docker buildx inspect --bootstrap

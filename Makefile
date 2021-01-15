# Makefile for building Litmus and its tools
# Reference Guide - https://www.gnu.org/software/make/manual/make.html

#
# Internal variables or constants.
# NOTE - These will be executed when any make target is invoked.
#
IS_DOCKER_INSTALLED = $(shell which docker >> /dev/null 2>&1; echo $$?)

# Docker info
DOCKER_REPO ?= litmuschaos
DOCKER_IMAGE ?= go-runner
DOCKER_TAG ?= ci

PACKAGES = $(shell go list ./... | grep -v '/vendor/')

.PHONY: all
all: deps gotasks build push trivy-check build-amd64 push-amd64

.PHONY: help
help:
	@echo ""
	@echo "Usage:-"
	@echo "\tmake all   -- [default] builds the litmus containers"
	@echo ""

.PHONY: deps
deps: _build_check_docker

_build_check_docker:
	@echo "------------------"
	@echo "--> Check the Docker deps" 
	@echo "------------------"
	@if [ $(IS_DOCKER_INSTALLED) -eq 1 ]; \
		then echo "" \
		&& echo "ERROR:\tdocker is not installed. Please install it before build." \
		&& echo "" \
		&& exit 1; \
		fi;

.PHONY: gotasks
gotasks: format lint unused-package-check

.PHONY: format
format:
	@echo "------------------"
	@echo "--> Running go fmt"
	@echo "------------------"
	@go fmt $(PACKAGES)

.PHONY: lint
lint:
	@echo "------------------"
	@echo "--> Running golint"
	@echo "------------------"
	@go get -u golang.org/x/lint/golint
	@golint $(PACKAGES)
	@echo "------------------"
	@echo "--> Running go vet"
	@echo "------------------"
	@go vet $(PACKAGES)

.PHONY: unused-package-check
unused-package-check:
	@echo "------------------"
	@echo "--> Check unused packages for the litmus-go"
	@echo "------------------"
	@tidy=$$(go mod tidy); \
	if [ -n "$${tidy}" ]; then \
		echo "go mod tidy checking failed!"; echo "$${tidy}"; echo; \
	fi

.PHONY: build
build: experiment-build image-build

.PHONY: experiment-build
experiment-build:
	@echo "------------------------------"
	@echo "--> Build experiment go binary" 
	@echo "------------------------------"
	@./build/go-multiarch-build.sh build/generate_go_binary

.PHONY: image-build
image-build:	
	@echo "-------------------------"
	@echo "--> Build go-runner image" 
	@echo "-------------------------"
	@sudo docker buildx build --file build/Dockerfile --progress plane --platform linux/arm64,linux/amd64 --no-cache --tag $(DOCKER_REPO)/$(DOCKER_IMAGE):$(DOCKER_TAG) .

.PHONY: build-amd64
build-amd64:

	@echo "------------------------------"
	@echo "--> Build experiment go binary" 
	@echo "------------------------------"
	@env GOOS=linux GOARCH=amd64 sh build/generate_go_binary
	@echo "-------------------------"
	@echo "--> Build go-runner image" 
	@echo "-------------------------"
	@sudo docker build --file build/Dockerfile --tag $(DOCKER_REPO)/$(DOCKER_IMAGE):$(DOCKER_TAG) . --build-arg TARGETARCH=amd64

.PHONY: push-amd64
push-amd64:

	@echo "------------------------------"
	@echo "--> Pushing image" 
	@echo "------------------------------"
	@sudo docker push $(DOCKER_REPO)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	
.PHONY: push
push: litmus-go-push

litmus-go-push:
	@echo "-------------------"
	@echo "--> go-runner image" 
	@echo "-------------------"
	REPONAME="$(DOCKER_REPO)" IMGNAME="$(DOCKER_IMAGE)" IMGTAG="$(DOCKER_TAG)" ./build/push
	
.PHONY: trivy-check
trivy-check:

	@echo "------------------------"
	@echo "---> Running Trivy Check"
	@echo "------------------------"
	@./trivy --exit-code 0 --severity HIGH --no-progress $(DOCKER_REPO)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	@./trivy --exit-code 0 --severity CRITICAL --no-progress $(DOCKER_REPO)/$(DOCKER_IMAGE):$(DOCKER_TAG)

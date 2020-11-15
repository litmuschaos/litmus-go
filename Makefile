# Makefile for building Litmus and its tools
# Reference Guide - https://www.gnu.org/software/make/manual/make.html

#
# Internal variables or constants.
# NOTE - These will be executed when any make target is invoked.
#
IS_DOCKER_INSTALLED = $(shell which docker >> /dev/null 2>&1; echo $$?)

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
	@echo "--> Check unused packages for the chaos-operator"
	@echo "------------------"
	@tidy=$$(go mod tidy); \
	if [ -n "$${tidy}" ]; then \
		echo "go mod tidy checking failed!"; echo "$${tidy}"; echo; \
	fi

.PHONY: build
build:

	@echo "------------------------------"
	@echo "--> Build experiment go binary" 
	@echo "------------------------------"
	@./build/go-multiarch-build.sh build/generate_go_binary
	@echo "-------------------------"
	@echo "--> Build go-runner image" 
	@echo "-------------------------"
	@sudo docker buildx build --file build/litmus-go/Dockerfile --progress plane --platform linux/arm64,linux/amd64 --no-cache --tag litmuschaos/go-runner:ci .

.PHONY: build-amd64
build-amd64:

	@echo "------------------------------"
	@echo "--> Build experiment go binary" 
	@echo "------------------------------"
	@env GOOS=linux GOARCH=amd64 sh build/generate_go_binary
	@echo "-------------------------"
	@echo "--> Build go-runner image" 
	@echo "-------------------------"
	@sudo docker build --file build/litmus-go/Dockerfile --tag litmuschaos/go-runner:ci . --build-arg TARGETARCH=amd64

.PHONY: push-amd64
push-amd64:

	@echo "------------------------------"
	@echo "--> Pushing image" 
	@echo "------------------------------"
	@sudo docker push litmuschaos/go-runner:ci
	
.PHONY: push
push: litmus-go-push

litmus-go-push:
	@echo "-------------------"
	@echo "--> go-runner image" 
	@echo "-------------------"
	REPONAME="litmuschaos" IMGNAME="go-runner" IMGTAG="ci" ./build/push
	
.PHONY: trivy-check
trivy-check:

	@echo "------------------------"
	@echo "---> Running Trivy Check"
	@echo "------------------------"
	@./trivy --exit-code 0 --severity HIGH --no-progress litmuschaos/go-runner:ci
	@./trivy --exit-code 0 --severity CRITICAL --no-progress litmuschaos/go-runner:ci


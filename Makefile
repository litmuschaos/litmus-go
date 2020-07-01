# Makefile for building Litmus and its tools
# Reference Guide - https://www.gnu.org/software/make/manual/make.html

#
# Internal variables or constants.
# NOTE - These will be executed when any make target is invoked.
#
IS_DOCKER_INSTALLED = $(shell which docker >> /dev/null 2>&1; echo $$?)

.PHONY: all
all: deps gotasks build push

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
	@echo "------------------"
	@echo "--> Build experiment go binary" 
	@echo "------------------"
	@sh build/generate_go_binary
	@echo "------------------"
	@echo "--> Build go-runner image" 
	@echo "------------------"
	sudo docker build . -f build/litmus-go/Dockerfile -t litmuschaos/go-runner:ci

.PHONY: push
push:
	@echo "------------------"
	@echo "--> go-runner image" 
	@echo "------------------"
	REPONAME="litmuschaos" IMGNAME="go-runner" IMGTAG="ci" ./build/push

.PHONY: security-checks
security-checks: trivy-security-check

trivy-security-check:
	@echo "------------------"
	@echo "--> Trivy Security Check"
	@echo "------------------"
	./trivy --exit-code 0 --severity HIGH --no-progress litmuschaos/litmus-go:ci
	./trivy --exit-code 1 --severity CRITICAL --no-progress litmuschaos/litmus-go:ci

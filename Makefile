# Makefile for building Litmus and its tools
# Reference Guide - https://www.gnu.org/software/make/manual/make.html

#
# Internal variables or constants.
# NOTE - These will be executed when any make target is invoked.
#
IS_DOCKER_INSTALLED = $(shell which docker >> /dev/null 2>&1; echo $$?)

.PHONY: all
all: deps go-build build push

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

PHONY: go-build
go-build: experiment-go-binary

experiment-go-binary:
	@echo "------------------"
	@echo "--> Build experiment go binary" 
	@echo "------------------"
	@sh hack/generate_go_binary

.PHONY: build
build: litmus-go-build

litmus-go-build:
	@echo "------------------"
	@echo "--> Build litmus-go image" 
	@echo "------------------"
	sudo docker build . -f build/litmus-go/Dockerfile -t shubh214/litmus-go:ci2

.PHONY: push
push: litmus-go-push

litmus-go-push:
	@echo "------------------"
	@echo "--> litmus-go image" 
	@echo "------------------"
	REPONAME="litmuschaos" IMGNAME="litmus-go" IMGTAG="ci" ./hack/push
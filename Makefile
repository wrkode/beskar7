# Basic Makefile for beskar7

# Go parameters
GOPATH:=$(shell go env GOPATH)
GOBIN=$(firstword $(subst :, ,${GOPATH}))/bin
GO ?= go

# Controller-gen tool
CONTROLLER_GEN = $(GOBIN)/controller-gen

# Image URL to use all building/pushing image targets
VERSION ?= v0.1.0-dev
IMAGE_REGISTRY ?= ghcr.io/wrkode
IMAGE_REPO ?= beskar7
IMG ?= $(IMAGE_REGISTRY)/$(IMAGE_REPO):$(VERSION)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:generateEmbeddedObjectMeta=true,maxDescLen=0"

# Build the manager binary
build:
	$(GO) build -o bin/manager cmd/manager/main.go

# Run code generators
generate:
	$(GO) generate ./...

# Install controller-gen
install-controller-gen:
	$(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

# Generate manifests e.g. CRDs, RBAC, and DeepCopy objects
manifests: install-controller-gen
	$(CONTROLLER_GEN) object:headerFile="./hack/boilerplate.go.txt" paths="./..."
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run tests
test:
	$(GO) test ./... -coverprofile cover.out

# Docker build for linux/amd64
docker-build:
	# Ensure you have a buildx builder configured that supports cross-compilation
	# e.g., docker buildx create --use
	docker buildx build --platform linux/amd64 -t $(IMG) --load .

# Docker push (uses IMG variable defined at the top)
docker-push:
	docker push $(IMG)

# Deploy to Kubernetes
# deploy: manifests
# 	kubectl apply -k config/default

.PHONY: build generate manifests test docker-build docker-push deploy install-controller-gen 
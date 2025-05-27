# Basic Makefile for beskar7

# Go parameters
GOPATH:=$(shell go env GOPATH)
GOBIN=$(firstword $(subst :, ,${GOPATH}))/bin
GO ?= go

# Controller-gen tool
CONTROLLER_GEN = $(GOBIN)/controller-gen

# Kustomize tool
KUSTOMIZE ?= kustomize

# Image URL to use all building/pushing image targets
VERSION ?= v0.2.4
IMAGE_REGISTRY ?= ghcr.io/wrkode/beskar7
IMAGE_REPO ?= beskar7
IMG ?= $(IMAGE_REGISTRY)/$(IMAGE_REPO):$(VERSION)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "generateEmbeddedObjectMeta=true,maxDescLen=0"

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
	$(MAKE) rbac crd

# Generate RBAC manifests
rbac:
	$(CONTROLLER_GEN) rbac:roleName=manager-role paths="./..." output:rbac:dir=config/rbac

# Generate CRD manifests
crd:
	$(CONTROLLER_GEN) crd:generateEmbeddedObjectMeta=true,maxDescLen=0 paths="./api/..." output:crd:artifacts:config=config/crd/bases

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

# Install CRDs into the cluster
install:
	$(MAKE) manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from the cluster
uninstall:
	$(MAKE) manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller to the cluster specified in ~/.kube/config
deploy:
	$(MAKE) manifests
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

# Undeploy controller from the cluster specified in ~/.kube/config
undeploy:
	$(KUSTOMIZE) build config/default | kubectl delete -f -

# Generate a single manifest file for a release
release-manifests:
	$(MAKE) manifests # Ensure CRDs and RBAC are up-to-date
	$(KUSTOMIZE) build config/default > beskar7-manifests-$(VERSION).yaml
	@echo "Release manifests generated: beskar7-manifests-$(VERSION).yaml"

.PHONY: build generate manifests test docker-build docker-push deploy install-controller-gen install uninstall undeploy rbac crd release-manifests 
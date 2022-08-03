GOLANGCILINTVERSION?=1.45.2
GOIMPORTSVERSION?=v0.1.8
GOXVERSION?=v1.0.1
GOFLAGS=-mod=vendor
CGO_ENABLED?=0
export GOFLAGS CGO_ENABLED

build: install-tools
	gox -output="bin/ally-{{.OS}}-{{.Arch}}" \
            -osarch="darwin/amd64 linux/amd64" .

go-vendor:
	go mod tidy
	go mod vendor
	go mod verify

lint: install-tools
	golangci-lint run

container: build
	docker build --no-cache -t techallylw/ally .

publish: container
	docker push techallylw/ally

install-tools:
ifeq (, $(shell which golangci-lint))
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v$(GOLANGCILINTVERSION)
endif
ifeq (, $(shell which goimports))
	GOFLAGS=-mod=readonly go install golang.org/x/tools/cmd/goimports@$(GOIMPORTSVERSION)
endif
ifeq (, $(shell which gox))
	GOFLAGS=-mod=readonly go install github.com/mitchellh/gox@$(GOXVERSION)
endif

GOLANGCILINTVERSION?=1.23.8
GOIMPORTSVERSION?=v0.1.2
GOXVERSION?=v1.0.1
GOFLAGS=-mod=readonly
CGO_ENABLED?=0
export GOFLAGS CGO_ENABLED

build:
	gox -output="bin/ally-{{.OS}}-{{.Arch}}" \
            -osarch="darwin/amd64 linux/amd64" .

vendor:
	go mod tidy
	go mod vendor
	go mod verify

lint:
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
	go get golang.org/x/tools/cmd/goimports@$(GOIMPORTSVERSION)
endif
ifeq (, $(shell which gox))
	go get github.com/mitchellh/gox@$(GOXVERSION)
endif

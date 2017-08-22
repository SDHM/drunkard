###Makefile for drunkard

CURDIR := $(shell pwd)

GO        := GO15VENDOREXPERIMENT="1" go
GOBUILD   := GOPATH=$(CURDIR)/_vendor:$(GOPATH) CGO_ENABLED=0 $(GO) build
GOTEST    := GOPATH=$(CURDIR)/_vendor:$(GOPATH) CGO_ENABLED=1 $(GO) test


build:
	$(GOBUILD)

clean:
	$(GO) clean -i ./...
	rm -rf *.out


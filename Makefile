CORES := $(shell nproc)

build:
	CGO_ENABLED=0 GOMAXPROCS=$(CORES) go build -p $(CORES) -ldflags="-s -w" -o kdiff

compress: build
	tar -cvzf kdiff.tgz ./kdiff

.PHONY: build compress
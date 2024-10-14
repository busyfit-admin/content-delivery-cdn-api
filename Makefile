SUBDIRS := $(wildcard */.)
REVISION := $(shell git rev-parse --short HEAD)
ARTIFACT := artifact-$(REVISION).tar.bz2


test: $(SUBDIRS)

build: $(SUBDIRS)

update: $(SUBDIRS)

deploy: $(SUBDIRS)

local: $(SUBDIRS)


$(SUBDIRS):
	$(MAKE) -C $@ $(MAKECMDGOALS)

.PHONY: test build local deploy $(SUBDIRS)
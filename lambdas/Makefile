SUBDIRS := $(wildcard */.)

test: $(SUBDIRS)

build: $(SUBDIRS)

update: ${SUBDIRS}

local: ${SUBDIRS}

deploy:

$(SUBDIRS):
	$(MAKE) -C $@ $(MAKECMDGOALS)

.PHONY: test build local update deploy $(SUBDIRS) 

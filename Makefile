SHELL := /bin/sh

ELM_SRC := src/Main.elm
DIST_DIR := dist
RESULTS_FILE := results.json
ELM_RAW := $(DIST_DIR)/dashboard.unminified.js
ELM_OUT := $(DIST_DIR)/dashboard.js
OPTIMIZE_SCRIPT := ./optimize.sh

.PHONY: all build build-dev build-with-results check check-services clean prepare-dist serve test validate

all: build

prepare-dist:
	mkdir -p $(DIST_DIR)
	cp index.html $(DIST_DIR)/
	cp src/styles.css $(DIST_DIR)/styles.css
	cp projects.json $(DIST_DIR)/projects.json
	cp $(RESULTS_FILE) $(DIST_DIR)/results.json

build: prepare-dist
	$(OPTIMIZE_SCRIPT) $(ELM_SRC) $(ELM_RAW) $(ELM_OUT)
	rm -f $(ELM_RAW)

build-dev: prepare-dist
	yarn -s elm make --debug $(ELM_SRC) --output=$(ELM_OUT)

build-with-results:
	$(MAKE) check-services
	$(MAKE) build

serve: build-dev
	python3 -m http.server 8010 --directory $(DIST_DIR)

validate:
	yarn validate:data

test:
	yarn test

check: validate test build

check-services:
	go run ./cmd/iiif-checker -results $(RESULTS_FILE)

clean:
	rm -rf $(DIST_DIR)

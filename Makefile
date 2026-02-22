MODULES=$(shell go list -f '{{.Dir}}/...' -m)

test:
	go test -timeout 1200s -race -cover $(MODULES)
.PHONY: test

update-testdata:
	umask 0022 && UPDATE=1 go test $(MODULES)
.PHONY: update-testdata

test-compile:
	go test -c -o /dev/null $(MODULES)
.PHONY: test-compile

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run -v $(MODULES)
.PHONY: lint

lint-fix: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --fix -v $(MODULES)
.PHONY: lint-fix

work-sync:
	go work sync
.PHONY: work-sync

tidy: $(MODULES:/...=.tidy) work-sync

%.tidy:
	cd $(basename $@) && go mod tidy
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

lint:
	go tool golangci-lint run -v $(MODULES)
.PHONY: lint

lint-fix:
	go tool golangci-lint run --fix -v $(MODULES)
.PHONY: lint-fix

work-sync:
	go work sync
.PHONY: work-sync

tidy: $(MODULES:/...=.tidy) work-sync

%.tidy:
	cd $(basename $@) && go mod tidy

bump: $(MODULES:/...=.bump) tidy

%.bump:
	cd $(basename $@) && go mod edit -json | jq --raw-output '.Require[] | select(.Path | contains("github.com/Azure/ARO-Tools") ) | .Path' | xargs -I{} go get {}@main
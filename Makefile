tidy:
	go mod tidy
.PHONY: tidy

test:
	go test -cover -race ./...
.PHONY: test

test-compile:
	go test -c -o /dev/null ./...
.PHONY: test-compile

lint:
	go tool golangci-lint run ./...
.PHONY: lint

lint-fix:
	go tool golangci-lint run --fix ./...
.PHONY: lint-fix

format:
	go tool golangci-lint fmt  ./.
.PHONY: format

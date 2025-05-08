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
	go tool github.com/daixiang0/gci write -s standard -s default -s 'prefix(k8s.io)' -s 'prefix(sigs.k8s.io)' -s 'prefix(github.com/Azure)' -s blank -s dot .
.PHONY: format

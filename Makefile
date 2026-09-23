BIN_NAME=sequencer

.PHONY: build
build:
	CGO_ENABLED=0 go build -v -o ${BIN_NAME} .

.PHONY: lint
lint:
	golangci-lint run --fix

.PHONY: test
test:
	go test -v ./...

.PHONY: test-coverage
test-coverage:
	go test -v -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

.PHONY: coverage-html
coverage-html: test-coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: coverage
coverage:
	go test -cover ./...

.PHONY: clean
clean:
	rm -f $(BIN_NAME)
	rm -rf coverage.out coverage.html

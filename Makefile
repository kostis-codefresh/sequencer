DIST_DIR=dist

.PHONY: build
build: build-sequencer build-web

.PHONY: build-sequencer
build-sequencer:
	CGO_ENABLED=0 go build -v -o ${DIST_DIR}/sequencer ./cmd/sequencer

.PHONY: build-web
build-web:
	CGO_ENABLED=0 go build -v -o ${DIST_DIR}/sequencer-web ./cmd/sequencer-web

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
	rm -rf $(DIST_DIR)
	rm -rf coverage.out coverage.html

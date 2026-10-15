BINARY := port-checker
.PHONY: build test lint run install clean
build:
	go build -o $(BINARY) .
test:
	go test ./...
lint:
	golangci-lint run ./...
run: build
	./$(BINARY)
install:
	go install .
clean:
	rm -f $(BINARY)

BIN := bin/bounty

.PHONY: build test vet check clean install

build:
	go build -o $(BIN) ./cmd/bounty

test:
	go test ./...

vet:
	go vet ./...

check: build vet test

# Installs the adapter where the engine looks for it.
install: build
	install -D -m 0755 $(BIN) $(HOME)/.config/iode/adapters/bounty

clean:
	rm -rf bin

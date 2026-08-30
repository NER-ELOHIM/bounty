BIN := bin/bounty

.PHONY: build test vet check clean install

build:
	go build -o $(BIN) ./cmd/bounty

test:
	go test ./...

vet:
	go vet ./...

check: build vet test

# Instala o adaptador onde o iode procura por ele.
install: build
	install -D -m 0755 $(BIN) $(HOME)/.config/iode/adapters/bounty

clean:
	rm -rf bin

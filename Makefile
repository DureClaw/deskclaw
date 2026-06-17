# deskclaw — desktop GUI node. Pure-Go, no CGo (built-in OS tooling).
BIN := deskclaw
DIST := dist

.PHONY: build all clean
build:
	go build -o $(BIN) .

# deskclaw targets the desktop OSes (Windows/macOS); Linux builds as a join-only stub.
all:
	mkdir -p $(DIST)
	GOOS=darwin  GOARCH=arm64 go build -o $(DIST)/deskclaw-darwin-arm64   .
	GOOS=darwin  GOARCH=amd64 go build -o $(DIST)/deskclaw-darwin-amd64   .
	GOOS=windows GOARCH=amd64 go build -o $(DIST)/deskclaw-windows-amd64.exe .
	GOOS=windows GOARCH=arm64 go build -o $(DIST)/deskclaw-windows-arm64.exe .
	GOOS=linux   GOARCH=amd64 go build -o $(DIST)/deskclaw-linux-amd64    .
	cd $(DIST) && shasum -a 256 deskclaw-* > SHA256SUMS

clean:
	rm -rf $(BIN) $(DIST)

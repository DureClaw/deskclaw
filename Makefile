# deskclaw — desktop GUI node. Pure-Go, no CGo (built-in OS tooling).
BIN := deskclaw
DIST := dist

.PHONY: build all clean app install-app icon
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

# macOS menu-bar app: DeskClaw.app = Swift shell + Go node + prebuilt axhelper (universal:
# arm64 + x86_64). Signed with your Apple Development identity when one is installed, so
# macOS permissions granted to DeskClaw.app survive rebuilds; otherwise ad-hoc ("-"), and a
# rebuild may need Accessibility / Screen Recording re-granted. Override: make app SIGN=…
SIGN ?= $(or $(shell security find-identity -p codesigning -v 2>/dev/null | sed -n 's/.*"\(Apple Development:[^"]*\)".*/\1/p' | head -1),-)
APP := $(DIST)/DeskClaw.app
MACOS := $(APP)/Contents/MacOS
TMP := $(DIST)/.app-build
app:
	rm -rf $(APP) $(TMP) && mkdir -p $(MACOS) $(TMP)
	mkdir -p $(APP)/Contents/Resources
	cp app/Info.plist $(APP)/Contents/
	cp app/DeskClaw.icns $(APP)/Contents/Resources/
	for a in arm64 x86_64; do \
	  swiftc -O -target $$a-apple-macos13 app/DeskClaw.swift -o $(TMP)/DeskClaw-$$a && \
	  swiftc -O -target $$a-apple-macos13 axhelper/axhelper.swift -o $(TMP)/axhelper-$$a || exit 1; \
	done
	GOOS=darwin GOARCH=arm64 go build -o $(TMP)/deskclaw-node-arm64 .
	GOOS=darwin GOARCH=amd64 go build -o $(TMP)/deskclaw-node-x86_64 .
	# the node is "deskclaw-node", not "deskclaw": APFS is case-insensitive, so it would
	# overwrite the DeskClaw shell executable
	for b in DeskClaw axhelper deskclaw-node; do lipo -create $(TMP)/$$b-arm64 $(TMP)/$$b-x86_64 -output $(MACOS)/$$b; done
	codesign --force --deep -s "$(SIGN)" $(APP)
	rm -rf $(TMP)
	cd $(DIST) && rm -f DeskClaw-macos.zip && ditto -c -k --keepParent DeskClaw.app DeskClaw-macos.zip \
	  && shasum -a 256 DeskClaw-macos.zip > DeskClaw-macos.zip.sha256   # setup.sh verifies it — publish both
	@echo "→ $(APP)  ($(DIST)/DeskClaw-macos.zip)"

# regenerate app/DeskClaw.icns from app/icon.swift
icon:
	rm -rf $(DIST)/.icon && swift app/icon.swift $(DIST)/.icon
	iconutil -c icns $(DIST)/.icon/DeskClaw.iconset -o app/DeskClaw.icns
	rm -rf $(DIST)/.icon

install-app: app
	rm -rf /Applications/DeskClaw.app
	cp -R $(APP) /Applications/
	open /Applications/DeskClaw.app

clean:
	rm -rf $(BIN) $(DIST)

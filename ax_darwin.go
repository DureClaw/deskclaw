//go:build darwin

// [AX] — universal element-level hands on macOS. Any app, by role/title/description/value
// instead of pixels, through the Accessibility API. Pure-Go can't call AX without CGo, so the
// Swift helper source is embedded in the binary and compiled once with swiftc (Xcode Command
// Line Tools) into the user cache dir — the single-file install story still holds.
package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed axhelper/axhelper.swift
var axSource []byte

var (
	axOnce sync.Once
	axBin  string
	axErr  error
)

func axSupported() bool { return true }

// axHelper compiles the embedded source on first use; the hash in the name rebuilds it
// whenever a new deskclaw ships a changed helper.
func axHelper() (string, error) {
	axOnce.Do(func() {
		if p := os.Getenv("DESK_AX_HELPER"); p != "" {
			axBin = p
			return
		}
		sum := sha256.Sum256(axSource)
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		dir := filepath.Join(base, "deskclaw")
		axBin = filepath.Join(dir, "axhelper-"+hex.EncodeToString(sum[:6]))
		if _, err := os.Stat(axBin); err == nil {
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			axErr = err
			return
		}
		src := axBin + ".swift"
		if err := os.WriteFile(src, axSource, 0o644); err != nil {
			axErr = err
			return
		}
		if out, err := exec.Command("swiftc", "-O", src, "-o", axBin).CombinedOutput(); err != nil {
			axErr = fmt.Errorf("building axhelper needs Xcode CLT (xcode-select --install): %v %s", err, clip(string(out), 600))
		}
	})
	return axBin, axErr
}

func axRun(args []string) (string, error) { return axRunFor(60*time.Second, args) }

// axRunFor is axRun with an explicit timeout — e.g. dumping thousands of chat rows (~50ms/row).
func axRunFor(timeout time.Duration, args []string) (string, error) {
	bin, err := axHelper()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(stderr.String()), err.Error()))
	}
	return strings.TrimSpace(string(out)), nil
}

// axOverlay shows the "automation running" border+banner until the returned stop is called
// (the helper also exits by itself when deskclaw dies or after 180s). Never blocks the task.
// send writes one visual-layer command line (virtual cursor, highlights) to the overlay.
func axOverlay(message string) (stop func(), send func([]byte)) {
	bin, err := axHelper()
	if err != nil {
		return func() {}, nil
	}
	cmd := exec.Command(bin, "overlay", message, "180")
	in, err := cmd.StdinPipe()
	if err != nil || cmd.Start() != nil {
		return func() {}, nil
	}
	time.Sleep(150 * time.Millisecond) // let the first frame draw
	return func() {
			_ = in.Close()
			_ = cmd.Process.Signal(os.Interrupt)
			_ = cmd.Wait()
		}, func(b []byte) {
			_, _ = in.Write(append(b, '\n'))
		}
}

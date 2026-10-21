//go:build !darwin

// [AX] element-level hands — macOS only for now. Windows UIAutomation is the next backend.
package main

import (
	"fmt"
	"runtime"
	"time"
)

func axSupported() bool { return false }

func axRunFor(_ time.Duration, args []string) (string, error) { return axRun(args) }

func axRun(args []string) (string, error) {
	return "", fmt.Errorf("[AX] not supported on %s yet (macOS only; Windows UIAutomation planned)", runtime.GOOS)
}

func axOverlay(message string) (stop func(), send func([]byte)) { return func() {}, nil }

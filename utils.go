// utils.go
package main

import (
	"log"
	"os"
)

// Logging functions with different levels
func SetupLogger(level string) {
	// Configure log output format
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Could be enhanced to support different log levels
	switch level {
	case "debug":
		log.SetOutput(os.Stdout)
	case "info":
		log.SetOutput(os.Stdout)
	case "warn":
		log.SetOutput(os.Stderr)
	case "error":
		log.SetOutput(os.Stderr)
	default:
		log.SetOutput(os.Stdout)
	}
}

func LogInfo(msg string) {
	log.Printf("[INFO] %s", msg)
}

func LogWarn(msg string) {
	log.Printf("[WARN] %s", msg)
}

func LogError(context string, err error) {
	log.Printf("[ERROR] %s: %v", context, err)
}

func LogDebug(msg string) {
	log.Printf("[DEBUG] %s", msg)
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// IsAlphaNumeric checks if a string contains only alphanumeric characters
func IsAlphaNumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// Min returns the smaller of two integers
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max returns the larger of two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

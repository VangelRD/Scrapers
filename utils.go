// utils.go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Logging functions
func SetupLogger(level string) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func LogInfo(msg string) {
	log.Printf("[INFO] %s", msg)
}

func LogDebug(msg string) {
	log.Printf("[DEBUG] %s", msg)
}

func LogWarn(msg string) {
	log.Printf("[WARN] %s", msg)
}

func LogError(context string, err error) {
	log.Printf("[ERROR] %s: %v", context, err)
}

// EnsureDir creates directory if it doesn't exist
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// IsAlphaNumeric checks if string is alphanumeric
func IsAlphaNumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// DownloadFile downloads a file with retry logic
func DownloadFile(url, filepath string, headers map[string]string, fetcher Fetcher, config Config) error {
	var lastErr error

	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		resp, err := fetcher.Get(url, headers)
		if err != nil {
			lastErr = err
			time.Sleep(config.RetryDelay * time.Duration(attempt+1))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			if resp.StatusCode == http.StatusNotFound {
				return lastErr // Don't retry 404s
			}
			time.Sleep(config.RetryDelay * time.Duration(attempt+1))
			continue
		}

		file, err := os.Create(filepath)
		if err != nil {
			lastErr = err
			time.Sleep(config.RetryDelay * time.Duration(attempt+1))
			continue
		}

		_, err = io.Copy(file, resp.Body)
		file.Close()
		if err != nil {
			lastErr = err
			os.Remove(filepath)
			time.Sleep(config.RetryDelay * time.Duration(attempt+1))
			continue
		}

		return nil
	}

	return fmt.Errorf("failed after %d attempts: %v", config.MaxRetries, lastErr)
}

// GetCommonHeaders returns common browser headers
func GetCommonHeaders() map[string]string {
	return map[string]string{
		"User-Agent":         "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36",
		"Accept-Language":    "en-US,en;q=0.9",
		"Sec-Ch-Ua":          `"Not=A?Brand";v="24", "Chromium";v="140"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"Linux"`,
	}
}

// ExtractBetween extracts string between start and end markers
func ExtractBetween(content, start, end string) string {
	startIdx := strings.Index(content, start)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(start)

	endIdx := strings.Index(content[startIdx:], end)
	if endIdx == -1 {
		return ""
	}

	return content[startIdx : startIdx+endIdx]
}

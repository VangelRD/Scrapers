// fetcher.go
package main

import (
	"net/http"
	"time"
)

// Fetcher abstracts HTTP requests for testability and flexibility
type Fetcher interface {
	Get(url string, headers map[string]string) (*http.Response, error)
}

// HTTPFetcher implements Fetcher using the standard net/http package
type HTTPFetcher struct {
	client *http.Client
}

// Get performs an HTTP GET request with the specified headers
func (f *HTTPFetcher) Get(url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Apply headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return f.client.Do(req)
}

// NewHTTPFetcher creates a new HTTPFetcher with the specified timeout
func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	return &HTTPFetcher{
		client: &http.Client{Timeout: timeout},
	}
}

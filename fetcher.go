// fetcher.go
package main

import (
	"net/http"
	"time"
)

// HTTPFetcher implements Fetcher using net/http
type HTTPFetcher struct {
	client *http.Client
}

func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	return &HTTPFetcher{
		client: &http.Client{Timeout: timeout},
	}
}

func (f *HTTPFetcher) Get(url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return f.client.Do(req)
}

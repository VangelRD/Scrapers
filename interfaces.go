// interfaces.go
package main

import "net/http"

// SiteScraper defines the interface that all site adapters must implement
type SiteScraper interface {
	// Core download methods
	DownloadAll() error
	DownloadBySlug(slug string) error

	// Site identification
	GetSiteName() string
}

// Fetcher abstracts HTTP requests for testability
type Fetcher interface {
	Get(url string, headers map[string]string) (*http.Response, error)
}

// Series represents a generic series/manga/manhwa
type Series struct {
	ID       string
	Slug     string
	Title    string
	CoverURL string
	Chapters []Chapter
}

// Chapter represents a generic chapter
type Chapter struct {
	ID        string
	Number    string
	Title     string
	URL       string
	ImageURLs []string
}

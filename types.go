// types.go
package main

import "time"

// Config holds all configuration options for the scraper
type Config struct {
	MaxWorkers        int           // Max concurrent image download workers
	MaxManhwaWorkers  int           // Max concurrent manhwa processors
	MaxChapterWorkers int           // Max concurrent chapter processors
	MaxImageWorkers   int           // Max concurrent image downloaders
	HTTPTimeout       time.Duration // HTTP request timeout
	PageBatchSize     int           // Number of pages to process concurrently
	MaxRetries        int           // Max retry attempts for failed requests
	RetryDelay        time.Duration // Base delay between retries
}

// SearchResponse represents the API response from /api/search
type SearchResponse struct {
	CurrentPage int           `json:"current_page"`
	Data        []ManhwaBrief `json:"data"`
	LastPage    int           `json:"last_page"`
	Total       int           `json:"total"`
}

// ManhwaBrief represents basic manhwa information
type ManhwaBrief struct {
	ID               int     `json:"id"`
	HID              string  `json:"hid"`
	Slug             string  `json:"slug"`
	Title            string  `json:"title"`
	LastChap         float64 `json:"last_chapter"`
	DefaultThumbnail string  `json:"default_thumbnail"`
}

// ChapterListResponse represents the API response from chapter-list endpoint
type ChapterListResponse struct {
	Data []ChapterBrief `json:"data"`
}

// ChapterBrief represents basic chapter information
type ChapterBrief struct {
	ID    int    `json:"id"`
	HID   string `json:"hid"`
	Chap  string `json:"chap"`
	Title string `json:"title"`
	Lang  string `json:"lang"`
}

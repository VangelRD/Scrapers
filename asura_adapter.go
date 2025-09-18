// asura_adapter.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AsuraAdapter implements SiteScraper for asuracomic.net
type AsuraAdapter struct {
	config      Config
	fetcher     Fetcher
	baseURL     string
	cdnURL      string
	seriesPool  *WorkerPool
	chapterPool *WorkerPool
	imagePool   *WorkerPool
}

// NewAsuraAdapter creates a new AsuraComic scraper
func NewAsuraAdapter(config Config) *AsuraAdapter {
	return &AsuraAdapter{
		config:      config,
		fetcher:     NewHTTPFetcher(config.HTTPTimeout),
		baseURL:     "https://asuracomic.net",
		cdnURL:      "https://gg.asuracomic.net",
		seriesPool:  NewWorkerPool(config.MaxSeriesWorkers),
		chapterPool: NewWorkerPool(config.MaxChapterWorkers),
		imagePool:   NewWorkerPool(config.MaxImageWorkers),
	}
}

// GetSiteName returns the site identifier
func (a *AsuraAdapter) GetSiteName() string {
	return "asura"
}

// DownloadAll downloads all series from AsuraComic
func (a *AsuraAdapter) DownloadAll() error {
	LogInfo("Fetching all series from AsuraComic...")
	series, err := a.getAllSeries()
	if err != nil {
		return err
	}

	LogInfo(fmt.Sprintf("Found %d series. Starting downloads...", len(series)))
	return a.downloadSeriesParallel(series)
}

// DownloadBySlug downloads a specific series
func (a *AsuraAdapter) DownloadBySlug(slug string) error {
	LogInfo(fmt.Sprintf("Downloading series: %s", slug))

	series := AsuraSeries{
		Slug:  slug,
		Title: slug,
	}

	return a.downloadSeries(series)
}

// AsuraSeries represents a series on AsuraComic
type AsuraSeries struct {
	Slug     string // Format: "title-hid"
	Title    string
	CoverURL string
	HID      string
}

// AsuraPage represents a single page/image in a chapter
type AsuraPage struct {
	Order int    `json:"order"`
	URL   string `json:"url"`
}

// getAllSeries fetches all series from AsuraComic
func (a *AsuraAdapter) getAllSeries() ([]AsuraSeries, error) {
	var allSeries []AsuraSeries
	var mu sync.Mutex
	var wg sync.WaitGroup

	// AsuraComic has max 20 pages
	const maxPages = 20

	for page := 1; page <= maxPages; page++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()

			url := fmt.Sprintf("%s/series?page=%d", a.baseURL, p)
			headers := a.getPageHeaders()

			resp, err := a.fetcher.Get(url, headers)
			if err != nil {
				LogError(fmt.Sprintf("Page %d", p), err)
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				LogError(fmt.Sprintf("Page %d read", p), err)
				return
			}

			series := a.parseSeriesFromHTML(string(body))

			mu.Lock()
			allSeries = append(allSeries, series...)
			LogInfo(fmt.Sprintf("Page %d: found %d series", p, len(series)))
			mu.Unlock()
		}(page)
	}

	wg.Wait()
	return allSeries, nil
}

// parseSeriesFromHTML extracts series from HTML
func (a *AsuraAdapter) parseSeriesFromHTML(html string) []AsuraSeries {
	var series []AsuraSeries
	seen := make(map[string]bool) // Avoid duplicates

	// Find all series links: <a href="series/[SLUG]-[HID]">
	// More specific pattern to match only series links
	re := regexp.MustCompile(`<a[^>]+href=["']series/([^"']+)["'][^>]*>`)
	matches := re.FindAllStringSubmatch(html, -1)

	LogDebug(fmt.Sprintf("Found %d potential series links", len(matches)))

	for _, match := range matches {
		if len(match) > 1 {
			fullSlug := match[1]

			// Skip if we've already seen this slug
			if seen[fullSlug] {
				continue
			}
			seen[fullSlug] = true

			// Extract title and HID from slug
			lastDashIdx := strings.LastIndex(fullSlug, "-")
			if lastDashIdx > 0 && lastDashIdx < len(fullSlug)-1 {
				title := strings.ReplaceAll(fullSlug[:lastDashIdx], "-", " ")
				title = strings.Title(strings.ToLower(title))
				hid := fullSlug[lastDashIdx+1:]

				// Validate HID format (should be alphanumeric)
				if IsAlphaNumeric(hid) && len(hid) >= 8 {
					series = append(series, AsuraSeries{
						Slug:  fullSlug,
						Title: title,
						HID:   hid,
					})
					LogDebug(fmt.Sprintf("Added series: %s (slug: %s, hid: %s)", title, fullSlug, hid))
				}
			}
		}
	}

	LogDebug(fmt.Sprintf("Parsed %d unique series from HTML", len(series)))
	return series
}

// downloadSeriesParallel downloads multiple series concurrently
func (a *AsuraAdapter) downloadSeriesParallel(series []AsuraSeries) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0

	for _, s := range series {
		wg.Add(1)
		go func(ser AsuraSeries) {
			defer wg.Done()

			a.seriesPool.Acquire()
			defer a.seriesPool.Release()

			err := a.downloadSeries(ser)

			mu.Lock()
			completed++
			if err != nil {
				LogError(fmt.Sprintf("[%d/%d] %s", completed, len(series), ser.Title), err)
			} else {
				LogInfo(fmt.Sprintf("[%d/%d] Completed: %s", completed, len(series), ser.Title))
			}
			mu.Unlock()
		}(s)
	}

	wg.Wait()
	return nil
}

// downloadSeries downloads a single series with all chapters
func (a *AsuraAdapter) downloadSeries(series AsuraSeries) error {
	// Get series page to extract cover
	seriesURL := fmt.Sprintf("%s/series/%s", a.baseURL, series.Slug)
	resp, err := a.fetcher.Get(seriesURL, a.getPageHeaders())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	html := string(body)

	// Extract and download cover
	coverURL := a.extractCoverURL(html)
	if coverURL != "" {
		a.downloadCover(series.Title, coverURL)
	}

	// Download chapters
	return a.downloadChapters(series)
}

// extractCoverURL finds the cover image URL from series page
func (a *AsuraAdapter) extractCoverURL(html string) string {
	// Look for cover image URLs - they're usually in gg.asuracomic.net and NOT -optimized or -thumbnail
	re := regexp.MustCompile(`https://gg\.asuracomic\.net/storage/media/\d+/[^"'\s]+\.webp`)
	matches := re.FindAllString(html, -1)

	for _, match := range matches {
		// Skip optimized, thumbnail, and conversion versions - we want the original
		if !strings.Contains(match, "-optimized") &&
			!strings.Contains(match, "-thumbnail") &&
			!strings.Contains(match, "-small") &&
			!strings.Contains(match, "/conversions/") {
			LogDebug(fmt.Sprintf("Found cover URL: %s", match))
			return match
		}
	}

	// Fallback: look for any media URL that's not in conversions folder
	re2 := regexp.MustCompile(`https://gg\.asuracomic\.net/storage/media/\d+/[^/]+\.webp`)
	matches2 := re2.FindAllString(html, -1)
	
	for _, match := range matches2 {
		if !strings.Contains(match, "/conversions/") {
			LogDebug(fmt.Sprintf("Found fallback cover URL: %s", match))
			return match
		}
	}

	LogDebug("No cover URL found")
	return ""
}

// downloadChapters downloads all chapters for a series sequentially to properly detect 404s
func (a *AsuraAdapter) downloadChapters(series AsuraSeries) error {
	consecutive404s := 0
	chapterNum := 0
	completed := 0

	LogInfo(fmt.Sprintf("[%s] Starting chapter discovery from chapter 0...", series.Title))

	for {
		// Check for 3 consecutive 404s
		if consecutive404s >= 3 {
			LogInfo(fmt.Sprintf("[%s] Stopped after 3 consecutive 404s", series.Title))
			break
		}

		// Download chapter sequentially for proper 404 detection
		err := a.downloadChapter(series, chapterNum)

		if err != nil {
			if strings.Contains(err.Error(), "404") {
				consecutive404s++
				LogDebug(fmt.Sprintf("[%s] Chapter %d: 404 (%d consecutive)", series.Title, chapterNum, consecutive404s))
			} else {
				LogDebug(fmt.Sprintf("[%s] Chapter %d failed: %v", series.Title, chapterNum, err))
				// For non-404 errors, continue but don't reset the counter
			}
		} else {
			consecutive404s = 0 // Reset on success
			completed++
			LogInfo(fmt.Sprintf("[%s] Chapter %d completed successfully", series.Title, chapterNum))
		}

		chapterNum++

		// Safety limit
		if chapterNum > 500 {
			LogWarn(fmt.Sprintf("[%s] Reached safety limit of 500 chapters", series.Title))
			break
		}

		// Small delay between chapter requests to be respectful
		time.Sleep(100 * time.Millisecond)
	}

	if completed > 0 {
		LogInfo(fmt.Sprintf("[%s] Successfully downloaded %d chapters", series.Title, completed))
	} else {
		LogWarn(fmt.Sprintf("[%s] No chapters were downloaded", series.Title))
	}

	return nil
}

// downloadChapter downloads a single chapter
func (a *AsuraAdapter) downloadChapter(series AsuraSeries, chapterNum int) error {
	// AsuraComic uses 0-based chapter numbering
	chapterURL := fmt.Sprintf("%s/series/%s/chapter/%d", a.baseURL, series.Slug, chapterNum)

	resp, err := a.fetcher.Get(chapterURL, a.getPageHeaders())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("404 for chapter %d", chapterNum)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	html := string(body)

	// Extract all image URLs from the JSON data
	imageURLs := a.extractImageURLs(html)
	if len(imageURLs) == 0 {
		return fmt.Errorf("no images found for chapter %d", chapterNum)
	}

	// Create chapter directory (use 1-based for consistency with output)
	displayChapterNum := chapterNum + 1
	chapterDir := filepath.Join("downloads", series.Title, fmt.Sprintf("chapter_%d", displayChapterNum))
	if err := EnsureDir(chapterDir); err != nil {
		return err
	}

	// Download images using the extracted URLs
	return a.downloadChapterImagesFromURLs(imageURLs, chapterDir)
}

// extractImageURLs extracts all image URLs from the chapter HTML using multiple methods
func (a *AsuraAdapter) extractImageURLs(html string) []string {
	// Method 1: Try to extract from JSON pages array
	urls := a.extractFromPagesJSON(html)
	if len(urls) > 0 {
		LogDebug(fmt.Sprintf("Method 1 (JSON): Extracted %d image URLs", len(urls)))
		return urls
	}

	// Method 2: Extract all image URLs directly from HTML
	urls = a.extractDirectImageURLs(html)
	if len(urls) > 0 {
		LogDebug(fmt.Sprintf("Method 2 (Direct): Extracted %d image URLs", len(urls)))
		return urls
	}

	LogDebug("No image URLs found using any method")
	return nil
}

// extractFromPagesJSON extracts URLs from the pages JSON array
func (a *AsuraAdapter) extractFromPagesJSON(html string) []string {
	// Look for the pages JSON array: "pages":[{"order":1,"url":"..."}...]
	re := regexp.MustCompile(`"pages":\s*\[(.*?)\]`)
	matches := re.FindStringSubmatch(html)

	if len(matches) < 2 {
		return nil
	}

	pagesJSON := "[" + matches[1] + "]"
	LogDebug(fmt.Sprintf("Found pages JSON: %s", pagesJSON[:min(200, len(pagesJSON))]))

	var pages []AsuraPage
	if err := json.Unmarshal([]byte(pagesJSON), &pages); err != nil {
		LogDebug(fmt.Sprintf("Failed to parse pages JSON: %v", err))
		return nil
	}

	var urls []string
	for _, page := range pages {
		if page.URL != "" {
			urls = append(urls, page.URL)
		}
	}

	return urls
}

// extractDirectImageURLs extracts image URLs directly by finding all optimized.webp URLs
func (a *AsuraAdapter) extractDirectImageURLs(html string) []string {
	// Find all optimized.webp URLs in order
	re := regexp.MustCompile(`https://gg\.asuracomic\.net/storage/media/\d+/conversions/\d+-optimized\.webp`)
	matches := re.FindAllString(html, -1)

	// Remove duplicates while preserving order
	seen := make(map[string]bool)
	var urls []string
	
	for _, url := range matches {
		if !seen[url] {
			seen[url] = true
			urls = append(urls, url)
		}
	}

	return urls
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// downloadChapterImagesFromURLs downloads images from a list of direct URLs
func (a *AsuraAdapter) downloadChapterImagesFromURLs(imageURLs []string, chapterDir string) error {
	downloadedCount := 0
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, imageURL := range imageURLs {
		wg.Add(1)
		go func(index int, url string) {
			defer wg.Done()

			a.imagePool.Acquire()
			defer a.imagePool.Release()

			// Save as 000.webp, 001.webp for consistency
			imagePath := filepath.Join(chapterDir, fmt.Sprintf("%03d.webp", index))

			time.Sleep(time.Duration(index*50) * time.Millisecond) // Stagger requests

			err := DownloadFile(url, imagePath, a.getImageHeaders(), a.fetcher, a.config)

			mu.Lock()
			if err != nil {
				LogDebug(fmt.Sprintf("Image %d failed: %v", index+1, err))
			} else {
				downloadedCount++
				LogDebug(fmt.Sprintf("Downloaded image %d/%d", index+1, len(imageURLs)))
			}
			mu.Unlock()
		}(i, imageURL)
	}

	wg.Wait()

	if downloadedCount == 0 {
		return fmt.Errorf("no images downloaded")
	}

	LogDebug(fmt.Sprintf("Successfully downloaded %d/%d images", downloadedCount, len(imageURLs)))
	return nil
}

// downloadCover downloads the cover image
func (a *AsuraAdapter) downloadCover(title, coverURL string) error {
	if coverURL == "" {
		return nil
	}

	dir := filepath.Join("downloads", title)
	if err := EnsureDir(dir); err != nil {
		return err
	}

	coverPath := filepath.Join(dir, "cover.webp")
	return DownloadFile(coverURL, coverPath, a.getImageHeaders(), a.fetcher, a.config)
}

// Header methods
func (a *AsuraAdapter) getPageHeaders() map[string]string {
	headers := GetCommonHeaders()
	headers["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"
	headers["Sec-Fetch-Site"] = "same-origin"
	headers["Sec-Fetch-Mode"] = "navigate"
	headers["Sec-Fetch-User"] = "?1"
	headers["Sec-Fetch-Dest"] = "document"
	headers["Referer"] = a.baseURL + "/"
	headers["Upgrade-Insecure-Requests"] = "1"
	return headers
}

func (a *AsuraAdapter) getImageHeaders() map[string]string {
	headers := GetCommonHeaders()
	headers["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"
	headers["Sec-Fetch-Site"] = "none"
	headers["Sec-Fetch-Mode"] = "navigate"
	headers["Sec-Fetch-User"] = "?1"
	headers["Sec-Fetch-Dest"] = "document"
	headers["Upgrade-Insecure-Requests"] = "1"
	return headers
}

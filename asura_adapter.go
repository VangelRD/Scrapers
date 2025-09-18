// asura_adapter.go
package main

import (
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

	// Find all series links: <a href="series/[SLUG]-[HID]">
	re := regexp.MustCompile(`<a href="series/([^"]+)">`)
	matches := re.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		if len(match) > 1 {
			fullSlug := match[1]

			// Extract title and HID from slug
			lastDashIdx := strings.LastIndex(fullSlug, "-")
			if lastDashIdx > 0 {
				title := fullSlug[:lastDashIdx]
				hid := fullSlug[lastDashIdx+1:]

				series = append(series, AsuraSeries{
					Slug:  fullSlug,
					Title: title,
					HID:   hid,
				})
			}
		}
	}

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
	// Look for /media/N/[random].webp pattern (not -optimized or -thumbnail)
	re := regexp.MustCompile(`/media/\d+/[^"'\s]+\.webp`)
	matches := re.FindAllString(html, -1)

	for _, match := range matches {
		// Skip optimized and thumbnail versions
		if !strings.Contains(match, "-optimized") &&
			!strings.Contains(match, "-thumbnail") &&
			!strings.Contains(match, "-small") {
			// Return full URL
			if strings.HasPrefix(match, "http") {
				return match
			}
			return a.baseURL + match
		}
	}

	return ""
}

// downloadChapters downloads all chapters for a series
func (a *AsuraAdapter) downloadChapters(series AsuraSeries) error {
	var wg sync.WaitGroup
	consecutive404s := 0
	chapterNum := 0

	for {
		// Check for 3 consecutive 404s
		if consecutive404s >= 3 {
			LogInfo(fmt.Sprintf("Stopped after 3 consecutive 404s for %s", series.Title))
			break
		}

		wg.Add(1)
		chNum := chapterNum

		go func(num int) {
			defer wg.Done()

			a.chapterPool.Acquire()
			defer a.chapterPool.Release()

			err := a.downloadChapter(series, num)
			if err != nil {
				if strings.Contains(err.Error(), "404") {
					consecutive404s++
				}
				LogDebug(fmt.Sprintf("Chapter %d failed: %v", num, err))
			} else {
				consecutive404s = 0 // Reset on success
				LogInfo(fmt.Sprintf("[%s] Chapter %d completed", series.Title, num))
			}
		}(chNum)

		chapterNum++

		// Wait for batch to complete before checking 404s
		if chapterNum%5 == 0 {
			wg.Wait()
			if consecutive404s >= 3 {
				break
			}
		}

		// Safety limit
		if chapterNum > 500 {
			LogWarn(fmt.Sprintf("Reached safety limit of 500 chapters for %s", series.Title))
			break
		}
	}

	wg.Wait()
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

	// Extract image base path
	imagePath := a.extractImagePath(html)
	if imagePath == "" {
		return fmt.Errorf("no image path found for chapter %d", chapterNum)
	}

	// Create chapter directory (use 1-based for consistency with output)
	displayChapterNum := chapterNum + 1
	chapterDir := filepath.Join("downloads", series.Title, fmt.Sprintf("chapter_%d", displayChapterNum))
	if err := EnsureDir(chapterDir); err != nil {
		return err
	}

	// Download images
	return a.downloadChapterImages(imagePath, chapterDir)
}

// extractImagePath extracts the base image path from chapter HTML
func (a *AsuraAdapter) extractImagePath(html string) string {
	// Look for pattern: gg.asuracomic.net/storage/media/[random]/conversions/0N-optimized.webp
	// Extract the [random] part
	re := regexp.MustCompile(`gg\.asuracomic\.net/storage/media/(\d+)/conversions/0\d+-optimized\.webp`)
	matches := re.FindStringSubmatch(html)

	if len(matches) > 1 {
		return matches[1]
	}

	// Try without domain
	re = regexp.MustCompile(`/storage/media/(\d+)/conversions/0\d+-optimized\.webp`)
	matches = re.FindStringSubmatch(html)

	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// downloadChapterImages downloads all images for a chapter
func (a *AsuraAdapter) downloadChapterImages(mediaPath, chapterDir string) error {
	consecutive404s := 0
	imageNum := 1 // AsuraComic images start from 01
	downloadedCount := 0

	for {
		if consecutive404s >= 3 {
			LogDebug(fmt.Sprintf("Stopped after 3 consecutive 404s. Downloaded %d images", downloadedCount))
			break
		}

		// Format: 01-optimized.webp, 02-optimized.webp, etc.
		imageURL := fmt.Sprintf("%s/storage/media/%s/conversions/%02d-optimized.webp",
			a.cdnURL, mediaPath, imageNum)

		// Save as 000.webp, 001.webp for consistency
		imagePath := filepath.Join(chapterDir, fmt.Sprintf("%03d.webp", imageNum-1))

		time.Sleep(50 * time.Millisecond) // Rate limiting

		err := DownloadFile(imageURL, imagePath, a.getImageHeaders(), a.fetcher, a.config)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				consecutive404s++
			}
			LogDebug(fmt.Sprintf("Image %d failed: %v", imageNum, err))
		} else {
			consecutive404s = 0
			downloadedCount++
		}

		imageNum++

		// Safety limit
		if imageNum > 200 {
			LogWarn("Reached safety limit of 200 images per chapter")
			break
		}
	}

	if downloadedCount == 0 {
		return fmt.Errorf("no images downloaded")
	}

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

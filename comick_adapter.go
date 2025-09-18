// comick_adapter.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ComickAdapter implements SiteScraper for comick.live
type ComickAdapter struct {
	config      Config
	fetcher     Fetcher
	baseURL     string
	cdnURL      string
	seriesPool  *WorkerPool
	chapterPool *WorkerPool
	imagePool   *WorkerPool
}

// NewComickAdapter creates a new Comick scraper
func NewComickAdapter(config Config) *ComickAdapter {
	return &ComickAdapter{
		config:      config,
		fetcher:     NewHTTPFetcher(config.HTTPTimeout),
		baseURL:     "https://comick.live",
		cdnURL:      "https://cdn1.comicknew.pictures",
		seriesPool:  NewWorkerPool(config.MaxSeriesWorkers),
		chapterPool: NewWorkerPool(config.MaxChapterWorkers),
		imagePool:   NewWorkerPool(config.MaxImageWorkers),
	}
}

// GetSiteName returns the site identifier
func (c *ComickAdapter) GetSiteName() string {
	return "comick"
}

// DownloadAll downloads all series
func (c *ComickAdapter) DownloadAll() error {
	LogInfo("Fetching all manhwas from Comick...")
	manhwas, err := c.getAllManhwas(0)
	if err != nil {
		return err
	}

	LogInfo(fmt.Sprintf("Found %d manhwas. Starting downloads...", len(manhwas)))
	return c.downloadManhwasParallel(manhwas)
}

// DownloadBySlug downloads a specific series
func (c *ComickAdapter) DownloadBySlug(slug string) error {
	LogInfo(fmt.Sprintf("Downloading manhwa: %s", slug))

	// Get manhwa info from API
	manhwa := ComickManhwa{Slug: slug, Title: slug}
	return c.downloadManhwa(manhwa)
}

// DownloadAfterID downloads manhwas with ID >= startID
func (c *ComickAdapter) DownloadAfterID(startID int) error {
	manhwas, err := c.getAllManhwas(startID)
	if err != nil {
		return err
	}

	LogInfo(fmt.Sprintf("Found %d manhwas after ID %d", len(manhwas), startID))
	return c.downloadManhwasParallel(manhwas)
}

// Comick-specific types
type ComickManhwa struct {
	ID        int    `json:"id"`
	HID       string `json:"hid"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Thumbnail string `json:"default_thumbnail"`
}

type ComickSearchResponse struct {
	CurrentPage int            `json:"current_page"`
	Data        []ComickManhwa `json:"data"`
	LastPage    int            `json:"last_page"`
}

type ComickChapter struct {
	ID    int    `json:"id"`
	HID   string `json:"hid"`
	Chap  string `json:"chap"`
	Title string `json:"title"`
	Lang  string `json:"lang"`
}

type ComickChapterResponse struct {
	Data []ComickChapter `json:"data"`
}

// getAllManhwas fetches all manhwas with optional ID filter
func (c *ComickAdapter) getAllManhwas(startID int) ([]ComickManhwa, error) {
	var allManhwas []ComickManhwa
	var mu sync.Mutex
	var wg sync.WaitGroup

	const maxPages = 3830
	pageLimiter := make(chan struct{}, 100) // Process 100 pages concurrently

	for page := 1; page <= maxPages; page++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()

			pageLimiter <- struct{}{}
			defer func() { <-pageLimiter }()

			url := fmt.Sprintf("%s/api/search?page=%d", c.baseURL, p)
			headers := c.getAPIHeaders()

			resp, err := c.fetcher.Get(url, headers)
			if err != nil {
				LogError(fmt.Sprintf("Page %d", p), err)
				return
			}
			defer resp.Body.Close()

			var searchResp ComickSearchResponse
			if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
				LogError(fmt.Sprintf("Page %d decode", p), err)
				return
			}

			var pageResults []ComickManhwa
			for _, m := range searchResp.Data {
				if startID == 0 || m.ID >= startID {
					pageResults = append(pageResults, m)
				}
			}

			mu.Lock()
			allManhwas = append(allManhwas, pageResults...)
			if p%50 == 0 {
				LogInfo(fmt.Sprintf("Processed page %d, total manhwas: %d", p, len(allManhwas)))
			}
			mu.Unlock()
		}(page)
	}

	wg.Wait()
	return allManhwas, nil
}

// downloadManhwasParallel downloads multiple manhwas concurrently
func (c *ComickAdapter) downloadManhwasParallel(manhwas []ComickManhwa) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0

	for _, manhwa := range manhwas {
		wg.Add(1)
		go func(m ComickManhwa) {
			defer wg.Done()

			c.seriesPool.Acquire()
			defer c.seriesPool.Release()

			err := c.downloadManhwa(m)

			mu.Lock()
			completed++
			if err != nil {
				LogError(fmt.Sprintf("[%d/%d] %s", completed, len(manhwas), m.Title), err)
			} else {
				LogInfo(fmt.Sprintf("[%d/%d] Completed: %s", completed, len(manhwas), m.Title))
			}
			mu.Unlock()
		}(manhwa)
	}

	wg.Wait()
	return nil
}

// downloadManhwa downloads a single manhwa with all chapters
func (c *ComickAdapter) downloadManhwa(manhwa ComickManhwa) error {
	// Download cover
	if manhwa.Thumbnail != "" {
		c.downloadCover(manhwa.Slug, manhwa.Thumbnail)
	}

	// Get chapters
	chapters, err := c.getChapterList(manhwa.Slug)
	if err != nil {
		return err
	}

	LogInfo(fmt.Sprintf("Found %d chapters for %s", len(chapters), manhwa.Title))

	// Download chapters
	return c.downloadChaptersParallel(manhwa.Slug, chapters)
}

// getChapterList gets all chapters with pagination
func (c *ComickAdapter) getChapterList(slug string) ([]ComickChapter, error) {
	var allChapters []ComickChapter
	page := 0
	consecutive404s := 0

	for {
		var url string
		if page == 0 {
			url = fmt.Sprintf("%s/api/comics/%s/chapter-list", c.baseURL, slug)
		} else {
			url = fmt.Sprintf("%s/api/comics/%s/chapter-list?page=%d", c.baseURL, slug, page)
		}

		resp, err := c.fetcher.Get(url, c.getAPIHeaders())
		if err != nil || resp.StatusCode == 404 {
			consecutive404s++
			if resp != nil {
				resp.Body.Close()
			}
			if consecutive404s >= 3 {
				break
			}
			page++
			continue
		}

		var chapterResp ComickChapterResponse
		err = json.NewDecoder(resp.Body).Decode(&chapterResp)
		resp.Body.Close()

		if err != nil {
			consecutive404s++
			if consecutive404s >= 3 {
				break
			}
			page++
			continue
		}

		consecutive404s = 0

		// Filter for English chapters
		for _, ch := range chapterResp.Data {
			if ch.Lang == "en" {
				allChapters = append(allChapters, ch)
			}
		}

		if len(chapterResp.Data) == 0 {
			break
		}

		page++
		if page > 100 { // Safety limit
			break
		}
	}

	return allChapters, nil
}

// downloadChaptersParallel downloads chapters concurrently
func (c *ComickAdapter) downloadChaptersParallel(slug string, chapters []ComickChapter) error {
	var wg sync.WaitGroup

	for _, chapter := range chapters {
		wg.Add(1)
		go func(ch ComickChapter) {
			defer wg.Done()

			c.chapterPool.Acquire()
			defer c.chapterPool.Release()

			if err := c.downloadChapter(slug, ch); err != nil {
				LogError(fmt.Sprintf("Chapter %s", ch.Chap), err)
			}
		}(chapter)
	}

	wg.Wait()
	return nil
}

// downloadChapter downloads a single chapter
func (c *ComickAdapter) downloadChapter(slug string, chapter ComickChapter) error {
	// Get image hash
	hash, err := c.getImageHash(slug, chapter.HID, chapter.Chap)
	if err != nil {
		return err
	}

	// Create directory
	chapterDir := filepath.Join("downloads", slug, fmt.Sprintf("chapter_%s", chapter.Chap))
	if err := EnsureDir(chapterDir); err != nil {
		return err
	}

	// Download images
	consecutive404s := 0
	for i := 0; i < 200; i++ {
		imageURL := fmt.Sprintf("%s/%s/0_%s/en/%s/%d.webp",
			c.cdnURL, slug, chapter.Chap, hash, i)
		imagePath := filepath.Join(chapterDir, fmt.Sprintf("%03d.webp", i))

		time.Sleep(50 * time.Millisecond) // Rate limiting

		err := DownloadFile(imageURL, imagePath, c.getImageHeaders(), c.fetcher, c.config)
		if err != nil {
			consecutive404s++
			if consecutive404s >= 3 {
				break
			}
			continue
		}
		consecutive404s = 0
	}

	return nil
}

// getImageHash extracts the hash for chapter images
func (c *ComickAdapter) getImageHash(slug, hid, chapter string) (string, error) {
	url := fmt.Sprintf("%s/comic/%s/%s-chapter-%s-en", c.baseURL, slug, hid, chapter)

	resp, err := c.fetcher.Get(url, c.getPageHeaders())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	bodyStr := string(body)

	// Try different extraction methods
	pattern := fmt.Sprintf("cdn1.comicknew.pictures/%s/0_%s/en/", slug, chapter)
	if hash := c.extractHashFromPattern(bodyStr, pattern); hash != "" {
		return hash, nil
	}

	// Try escaped patterns
	pattern = fmt.Sprintf("cdn1.comicknew.pictures\\/%s\\/0_%s\\/en\\/", slug, chapter)
	if hash := c.extractHashFromPattern(bodyStr, pattern); hash != "" {
		return hash, nil
	}

	return "", fmt.Errorf("hash not found for chapter %s", chapter)
}

// extractHashFromPattern extracts hash from URL pattern
func (c *ComickAdapter) extractHashFromPattern(content, pattern string) string {
	start := strings.Index(content, pattern)
	if start == -1 {
		// Try without backslashes
		pattern = strings.ReplaceAll(pattern, "\\", "")
		start = strings.Index(content, pattern)
	}

	if start != -1 {
		start += len(pattern)
		end := strings.IndexAny(content[start:], "/\"'\\")
		if end != -1 {
			hash := content[start : start+end]
			if len(hash) == 8 && IsAlphaNumeric(hash) {
				return hash
			}
		}
	}
	return ""
}

// downloadCover downloads the cover image
func (c *ComickAdapter) downloadCover(slug, coverURL string) error {
	if coverURL == "" {
		return nil
	}

	dir := filepath.Join("downloads", slug)
	if err := EnsureDir(dir); err != nil {
		return err
	}

	ext := ".webp"
	if strings.Contains(coverURL, ".jpg") {
		ext = ".jpg"
	} else if strings.Contains(coverURL, ".png") {
		ext = ".png"
	}

	coverPath := filepath.Join(dir, "cover"+ext)
	return DownloadFile(coverURL, coverPath, c.getImageHeaders(), c.fetcher, c.config)
}

// Header methods
func (c *ComickAdapter) getAPIHeaders() map[string]string {
	headers := GetCommonHeaders()
	headers["Accept"] = "*/*"
	headers["Sec-Fetch-Site"] = "same-origin"
	headers["Sec-Fetch-Mode"] = "cors"
	headers["Sec-Fetch-Dest"] = "empty"
	return headers
}

func (c *ComickAdapter) getPageHeaders() map[string]string {
	headers := GetCommonHeaders()
	headers["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	headers["Sec-Fetch-Site"] = "same-origin"
	headers["Sec-Fetch-Mode"] = "navigate"
	headers["Sec-Fetch-Dest"] = "document"
	return headers
}

func (c *ComickAdapter) getImageHeaders() map[string]string {
	headers := GetCommonHeaders()
	headers["Accept"] = "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"
	headers["Sec-Fetch-Site"] = "cross-site"
	headers["Sec-Fetch-Mode"] = "no-cors"
	headers["Sec-Fetch-Dest"] = "image"
	headers["Referer"] = "https://comick.live/"
	return headers
}

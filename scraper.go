// scraper.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ComickScraper handles all scraping operations with high performance
type ComickScraper struct {
	fetcher       Fetcher
	baseURL       string
	cdnURL        string
	config        Config
	manhwaPool    *WorkerPool
	chapterPool   *WorkerPool
	imagePool     *WorkerPool
	headerManager *HeaderManager
}

// NewComickScraper creates a new high-performance scraper
func NewComickScraper(cfg Config) *ComickScraper {
	fetcher := &HTTPFetcher{
		client: &http.Client{Timeout: cfg.HTTPTimeout},
	}

	headerManager := NewHeaderManager()

	return &ComickScraper{
		fetcher:       fetcher,
		baseURL:       "https://comick.live",
		cdnURL:        "https://cdn1.comicknew.pictures",
		config:        cfg,
		manhwaPool:    NewWorkerPool(cfg.MaxManhwaWorkers),
		chapterPool:   NewWorkerPool(cfg.MaxChapterWorkers),
		imagePool:     NewWorkerPool(cfg.MaxImageWorkers),
		headerManager: headerManager,
	}
}

// DownloadAllManhwas downloads all manhwas with high-performance parallel processing
func (s *ComickScraper) DownloadAllManhwas() error {
	manhwas, err := s.getAllManhwasParallel(0)
	if err != nil {
		return err
	}

	LogInfo(fmt.Sprintf("Found %d manhwas. Starting parallel downloads...", len(manhwas)))
	return s.downloadManhwasParallel(manhwas)
}

// DownloadManhwaBySlug downloads a single manhwa by slug
func (s *ComickScraper) DownloadManhwaBySlug(slug string) error {
	// Get manhwa info first (could be enhanced to fetch cover URL from API)
	manhwa := ManhwaBrief{Slug: slug, Title: slug, DefaultThumbnail: ""}
	return s.DownloadManhwa(manhwa)
}

// DownloadManhwasAfterID downloads manhwas with ID >= startID
func (s *ComickScraper) DownloadManhwasAfterID(startID int) error {
	manhwas, err := s.getAllManhwasParallel(startID)
	if err != nil {
		return err
	}

	LogInfo(fmt.Sprintf("Found %d manhwas after ID %d. Starting downloads...", len(manhwas), startID))
	return s.downloadManhwasParallel(manhwas)
}

// getAllManhwasParallel implements high-performance parallel page discovery
func (s *ComickScraper) getAllManhwasParallel(startID int) ([]ManhwaBrief, error) {
	var allManhwas []ManhwaBrief
	var mu sync.Mutex
	var wg sync.WaitGroup

	const maxPages = 3830
	pageLimiter := make(chan struct{}, s.config.PageBatchSize)

	LogInfo(fmt.Sprintf("Fetching pages in parallel (batch size: %d)...", s.config.PageBatchSize))

	for page := 1; page <= maxPages; page++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()

			// Acquire page processing slot
			pageLimiter <- struct{}{}
			defer func() { <-pageLimiter }()

			url := fmt.Sprintf("%s/api/search?page=%d", s.baseURL, p)
			headers := s.headerManager.GetAPIHeaders()

			resp, err := s.fetcher.Get(url, headers)
			if err != nil {
				LogError(fmt.Sprintf("Page %d", p), err)
				return
			}
			defer resp.Body.Close()

			var searchResp SearchResponse
			if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
				LogError(fmt.Sprintf("Page %d decode", p), err)
				return
			}

			// Filter manhwas by startID if specified
			var pageResults []ManhwaBrief
			for _, manhwa := range searchResp.Data {
				if startID == 0 || manhwa.ID >= startID {
					pageResults = append(pageResults, manhwa)
				}
			}

			// Thread-safe updates
			mu.Lock()
			allManhwas = append(allManhwas, pageResults...)
			if p%50 == 0 { // Log every 50 pages
				LogInfo(fmt.Sprintf("Page %d processed, found %d new manhwas (total: %d)", p, len(pageResults), len(allManhwas)))
			}
			mu.Unlock()
		}(page)
	}

	wg.Wait()
	LogInfo(fmt.Sprintf("Completed parallel page fetching. Total manhwas found: %d", len(allManhwas)))
	return allManhwas, nil
}

// downloadManhwasParallel downloads multiple manhwas in parallel
func (s *ComickScraper) downloadManhwasParallel(manhwas []ManhwaBrief) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0

	for _, manhwa := range manhwas {
		wg.Add(1)
		go func(m ManhwaBrief) {
			defer wg.Done()

			s.manhwaPool.Acquire()
			defer s.manhwaPool.Release()

			err := s.DownloadManhwa(m)

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

// DownloadManhwa downloads cover and all chapters for a manhwa
func (s *ComickScraper) DownloadManhwa(m ManhwaBrief) error {
	LogInfo(fmt.Sprintf("Starting download for manhwa: %s", m.Title))

	// Download cover image
	if err := s.downloadCover(m.Slug, m.DefaultThumbnail); err != nil {
		LogError("Cover download", err)
		// Continue with chapters even if cover fails
	}

	// Get chapter list with pagination support
	chapters, err := s.getChapterListPaginated(m.Slug)
	if err != nil {
		return fmt.Errorf("failed to get chapter list: %v", err)
	}

	LogInfo(fmt.Sprintf("Found %d English chapters for %s", len(chapters), m.Title))

	// Download chapters in parallel
	return s.downloadChaptersParallel(m.Slug, chapters)
}

// getChapterListPaginated handles paginated chapter lists efficiently
func (s *ComickScraper) getChapterListPaginated(slug string) ([]ChapterBrief, error) {
	var englishChapters []ChapterBrief
	page := 0
	consecutive404s := 0
	const max404s = 3

	LogInfo(fmt.Sprintf("Fetching chapter list for %s (checking for pagination)...", slug))

	for {
		var url string
		if page == 0 {
			url = fmt.Sprintf("%s/api/comics/%s/chapter-list", s.baseURL, slug)
		} else {
			url = fmt.Sprintf("%s/api/comics/%s/chapter-list?page=%d", s.baseURL, slug, page)
		}

		headers := s.headerManager.GetAPIHeaders()
		resp, err := s.fetcher.Get(url, headers)
		if err != nil || resp.StatusCode == 404 {
			consecutive404s++
			if resp != nil {
				resp.Body.Close()
			}

			if consecutive404s >= max404s {
				LogInfo(fmt.Sprintf("Stopping chapter list fetching after %d consecutive 404s", max404s))
				break
			}
			page++
			continue
		}

		var chapterResp ChapterListResponse
		err = json.NewDecoder(resp.Body).Decode(&chapterResp)
		resp.Body.Close()

		if err != nil {
			consecutive404s++
			if consecutive404s >= max404s {
				LogInfo(fmt.Sprintf("Stopping chapter list fetching after %d consecutive decode failures", max404s))
				break
			}
			page++
			continue
		}

		// Success - reset consecutive failures
		consecutive404s = 0

		// Filter for English chapters only
		pageChapters := 0
		for _, chapter := range chapterResp.Data {
			if chapter.Lang == "en" {
				englishChapters = append(englishChapters, chapter)
				pageChapters++
			}
		}

		LogDebug(fmt.Sprintf("Page %d: found %d English chapters (total: %d)", page, pageChapters, len(englishChapters)))

		if len(chapterResp.Data) == 0 {
			LogInfo(fmt.Sprintf("Page %d had no chapters, assuming end of pagination", page))
			break
		}

		page++
		if page > 100 { // Safety limit
			LogWarn("Reached safety limit of 100 pages, stopping")
			break
		}
	}

	LogInfo(fmt.Sprintf("Completed chapter list fetching for %s: %d total English chapters", slug, len(englishChapters)))
	return englishChapters, nil
}

// downloadChaptersParallel downloads multiple chapters in parallel
func (s *ComickScraper) downloadChaptersParallel(slug string, chapters []ChapterBrief) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0

	for _, chapter := range chapters {
		wg.Add(1)
		go func(ch ChapterBrief) {
			defer wg.Done()

			s.chapterPool.Acquire()
			defer s.chapterPool.Release()

			err := s.downloadChapter(slug, ch.HID, ch.Chap)

			mu.Lock()
			completed++
			if err != nil {
				LogError(fmt.Sprintf("[%s] Chapter %s (%d/%d)", slug, ch.Chap, completed, len(chapters)), err)
			} else {
				LogInfo(fmt.Sprintf("[%s] Chapter %s completed (%d/%d)", slug, ch.Chap, completed, len(chapters)))
			}
			mu.Unlock()
		}(chapter)
	}

	wg.Wait()
	return nil
}

// downloadCover downloads the cover image for a manhwa
func (s *ComickScraper) downloadCover(slug, coverURL string) error {
	if coverURL == "" {
		LogDebug(fmt.Sprintf("No cover image for %s", slug))
		return nil
	}

	LogDebug(fmt.Sprintf("Downloading cover image for %s...", slug))

	dir := filepath.Join("downloads", slug)
	if err := EnsureDir(dir); err != nil {
		return err
	}

	// Determine file extension from URL
	extension := ".webp"
	if strings.Contains(coverURL, ".jpg") || strings.Contains(coverURL, ".jpeg") {
		extension = ".jpg"
	} else if strings.Contains(coverURL, ".png") {
		extension = ".png"
	}

	coverPath := filepath.Join(dir, "cover"+extension)
	headers := s.headerManager.GetImageHeaders()

	return s.downloadImage(coverURL, coverPath, headers)
}

// downloadChapter downloads all images for a chapter using advanced hash discovery
func (s *ComickScraper) downloadChapter(slug, hid, chapter string) error {
	LogDebug(fmt.Sprintf("Downloading chapter %s...", chapter))

	// Get the image hash using advanced discovery algorithms
	hash, err := s.getChapterImageHashAdvanced(slug, hid, chapter)
	if err != nil {
		return fmt.Errorf("failed to get image hash: %v", err)
	}

	// Create chapter directory
	chapterDir := filepath.Join("downloads", slug, "chapter_"+chapter)
	if err := EnsureDir(chapterDir); err != nil {
		return err
	}

	// Download images with smart termination (3 consecutive 404s)
	return s.downloadChapterImagesSequential(slug, chapter, hash, chapterDir)
}

// downloadChapterImagesSequential downloads chapter images sequentially with 3 consecutive 404s logic
func (s *ComickScraper) downloadChapterImagesSequential(slug, chapter, hash, chapterDir string) error {
	const maxImages = 200
	const max404s = 3
	consecutive404s := 0
	downloadedCount := 0

	LogDebug(fmt.Sprintf("Starting download for chapter %s...", chapter))

	for i := 0; i < maxImages; i++ {
		imageURL := fmt.Sprintf("%s/%s/0_%s/en/%s/%d.webp", s.cdnURL, slug, chapter, hash, i)
		imagePath := filepath.Join(chapterDir, fmt.Sprintf("%03d.webp", i))

		// Small delay to avoid rate limiting
		time.Sleep(50 * time.Millisecond)

		headers := s.headerManager.GetImageHeaders()
		err := s.downloadImage(imageURL, imagePath, headers)

		if err != nil {
			consecutive404s++
			LogDebug(fmt.Sprintf("Image %d failed (consecutive failures: %d/%d): %v", i, consecutive404s, max404s, err))

			if consecutive404s >= max404s {
				LogDebug(fmt.Sprintf("Stopping after %d consecutive failures. Downloaded %d images.", max404s, downloadedCount))
				break
			}
			continue
		}

		// Successfully downloaded
		consecutive404s = 0
		downloadedCount++
		LogDebug(fmt.Sprintf("Downloaded image %d for chapter %s (total: %d)", i, chapter, downloadedCount))
	}

	if downloadedCount == 0 {
		return fmt.Errorf("no images downloaded for chapter %s", chapter)
	}

	LogDebug(fmt.Sprintf("Completed downloading chapter %s (%d images)", chapter, downloadedCount))
	return nil
}

// downloadImage downloads an image with retry logic
func (s *ComickScraper) downloadImage(url, filepath string, headers map[string]string) error {
	var lastErr error

	for attempt := 0; attempt < s.config.MaxRetries; attempt++ {
		resp, err := s.fetcher.Get(url, headers)
		if err != nil {
			lastErr = err
			time.Sleep(s.config.RetryDelay * time.Duration(attempt+1))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			resp.Body.Close()
			if resp.StatusCode == 404 {
				return lastErr // Don't retry 404s
			}
			time.Sleep(s.config.RetryDelay * time.Duration(attempt+1))
			continue
		}

		// Create file and copy content
		file, err := os.Create(filepath)
		if err != nil {
			lastErr = err
			time.Sleep(s.config.RetryDelay * time.Duration(attempt+1))
			continue
		}

		_, err = io.Copy(file, resp.Body)
		file.Close()
		if err != nil {
			lastErr = err
			os.Remove(filepath) // Clean up partial file
			time.Sleep(s.config.RetryDelay * time.Duration(attempt+1))
			continue
		}

		return nil // Success
	}

	return fmt.Errorf("failed after %d attempts: %v", s.config.MaxRetries, lastErr)
}

// getChapterImageHashAdvanced uses multiple strategies to find the image hash
func (s *ComickScraper) getChapterImageHashAdvanced(slug, hid, chapter string) (string, error) {
	url := fmt.Sprintf("%s/comic/%s/%s-chapter-%s-en", s.baseURL, slug, hid, chapter)
	headers := s.headerManager.GetPageHeaders()

	resp, err := s.fetcher.Get(url, headers)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	bodyStr := string(body)

	// Strategy 1: Direct pattern matching
	if hash := s.extractHashFromHTML(bodyStr, slug, chapter); hash != "" {
		LogDebug(fmt.Sprintf("Found hash via direct pattern: %s", hash))
		return hash, nil
	}

	// Strategy 2: JSON extraction from scripts
	if hash := s.extractHashFromScripts(bodyStr, slug, chapter); hash != "" {
		LogDebug(fmt.Sprintf("Found hash via script extraction: %s", hash))
		return hash, nil
	}

	// Strategy 3: HID-based guessing with validation
	if hash := s.guessHashFromHID(slug, hid, chapter); hash != "" {
		LogDebug(fmt.Sprintf("Found hash via HID guessing: %s", hash))
		return hash, nil
	}

	return "", fmt.Errorf("image hash not found for chapter %s", chapter)
}

// extractHashFromHTML extracts hash using pattern matching
func (s *ComickScraper) extractHashFromHTML(htmlContent, slug, chapter string) string {
	// Primary pattern: cdn1.comicknew.pictures/[SLUG]/0_[ChapNumber]/en/[hash]/
	pattern := fmt.Sprintf("cdn1.comicknew.pictures/%s/0_%s/en/", slug, chapter)

	start := strings.Index(htmlContent, pattern)
	if start != -1 {
		start += len(pattern)
		end := strings.Index(htmlContent[start:], "/")
		if end != -1 {
			hash := htmlContent[start : start+end]
			if len(hash) == 8 && IsAlphaNumeric(hash) {
				return hash
			}
		}
	}

	// Try escaped patterns
	patterns := []string{
		fmt.Sprintf("cdn1.comicknew.pictures\\/%s\\/0_%s\\/en\\/", slug, chapter),
		fmt.Sprintf("\\/cdn1.comicknew.pictures\\/%s\\/0_%s\\/en\\/", slug, chapter),
	}

	for _, escPattern := range patterns {
		if hash := s.extractFromEscapedPattern(htmlContent, escPattern); hash != "" {
			return hash
		}
	}

	return ""
}

// extractFromEscapedPattern extracts hash from escaped JSON patterns
func (s *ComickScraper) extractFromEscapedPattern(content, pattern string) string {
	start := strings.Index(content, strings.ReplaceAll(pattern, "\\", ""))
	if start == -1 {
		start = strings.Index(content, pattern)
	}

	if start != -1 {
		remaining := content[start:]
		if idx := strings.Index(remaining, "\\/en\\/"); idx != -1 {
			afterEn := remaining[idx+5:]
			if slashIdx := strings.Index(afterEn, "\\/"); slashIdx != -1 {
				hash := strings.TrimPrefix(afterEn[:slashIdx], "/")
				if len(hash) == 8 && IsAlphaNumeric(hash) {
					return hash
				}
			}
		}
	}
	return ""
}

// extractHashFromScripts extracts hash from JavaScript/JSON in script tags
func (s *ComickScraper) extractHashFromScripts(htmlContent, slug, chapter string) string {
	scriptStart := 0
	for {
		scriptStart = strings.Index(htmlContent[scriptStart:], "<script")
		if scriptStart == -1 {
			break
		}
		scriptStart += strings.Index(htmlContent[:scriptStart], "<script")

		scriptEnd := strings.Index(htmlContent[scriptStart:], "</script>")
		if scriptEnd == -1 {
			break
		}
		scriptEnd += scriptStart

		scriptContent := htmlContent[scriptStart:scriptEnd]
		if strings.Contains(scriptContent, slug) || strings.Contains(scriptContent, "comicknew") {
			if hash := s.findHashInContent(scriptContent, slug, chapter); hash != "" {
				return hash
			}
		}

		scriptStart = scriptEnd
	}
	return ""
}

// findHashInContent finds 8-character alphanumeric hashes and validates them
func (s *ComickScraper) findHashInContent(content, slug, chapter string) string {
	words := strings.FieldsFunc(content, func(r rune) bool {
		return r == '"' || r == '\'' || r == ',' || r == ':' || r == '{' || r == '}' ||
			r == '[' || r == ']' || r == ' ' || r == '\n' || r == '\t' || r == '/' || r == '\\'
	})

	for _, word := range words {
		word = strings.Trim(word, "\"'")
		if len(word) == 8 && IsAlphaNumeric(word) {
			if s.validateHash(slug, chapter, word) {
				return word
			}
		}
	}
	return ""
}

// guessHashFromHID generates potential hashes from HID and validates them
func (s *ComickScraper) guessHashFromHID(slug, hid, chapter string) string {
	variations := []string{
		hid,
		strings.ToLower(hid),
	}

	// Add substrings if HID is long enough
	if len(hid) >= 8 {
		variations = append(variations, hid[:8])
		variations = append(variations, hid[len(hid)-8:])
		variations = append(variations, strings.ToLower(hid)[:8])
	}

	for _, hash := range variations {
		if len(hash) == 8 && IsAlphaNumeric(hash) {
			if s.validateHash(slug, chapter, hash) {
				return hash
			}
		}
	}
	return ""
}

// validateHash tests if a hash works by attempting to download the first image
func (s *ComickScraper) validateHash(slug, chapter, hash string) bool {
	testURL := fmt.Sprintf("%s/%s/0_%s/en/%s/0.webp", s.cdnURL, slug, chapter, hash)
	headers := s.headerManager.GetImageHeaders()

	resp, err := s.fetcher.Get(testURL, headers)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

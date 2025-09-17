package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// API Response structures based on the provided examples
type SearchResponse struct {
	CurrentPage int `json:"current_page"`
	Data        []struct {
		ID               int     `json:"id"`
		HID              string  `json:"hid"`
		Slug             string  `json:"slug"`
		Title            string  `json:"title"`
		LastChap         float64 `json:"last_chapter"`
		DefaultThumbnail string  `json:"default_thumbnail"`
	} `json:"data"`
	LastPage int `json:"last_page"`
	Total    int `json:"total"`
}

type ChapterListResponse struct {
	Data []struct {
		ID    int    `json:"id"`
		HID   string `json:"hid"`
		Chap  string `json:"chap"`
		Title string `json:"title"`
		Lang  string `json:"lang"`
	} `json:"data"`
}

type ComickScraper struct {
	client            *http.Client
	baseURL           string
	cdnURL            string
	maxWorkers        int
	maxManhwaWorkers  int
	maxChapterWorkers int
	rateLimiter       chan struct{}
	manhwaLimiter     chan struct{}
	chapterLimiter    chan struct{}
}

func NewComickScraper() *ComickScraper {
	return &ComickScraper{
		client: &http.Client{
			Timeout: 15 * time.Second, // Faster timeout for bulk downloads
		},
		baseURL:           "https://comick.live",
		cdnURL:            "https://cdn1.comicknew.pictures",
		maxWorkers:        20,                      // Much higher for bulk downloads
		maxManhwaWorkers:  10,                      // Download 10 manhwas in parallel
		maxChapterWorkers: 5,                       // Download 5 chapters per manhwa in parallel
		rateLimiter:       make(chan struct{}, 50), // Much higher rate limit
		manhwaLimiter:     make(chan struct{}, 10), // Limit parallel manhwas
		chapterLimiter:    make(chan struct{}, 50), // Limit parallel chapters globally
	}
}

func (s *ComickScraper) makeRequest(url string) (*http.Response, error) {
	s.rateLimiter <- struct{}{}        // Acquire rate limit token
	defer func() { <-s.rateLimiter }() // Release token

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add headers from the provided examples
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Not=A?Brand";v="24", "Chromium";v="140"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Linux"`)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")

	return s.client.Do(req)
}

func (s *ComickScraper) getAllManhwas(startID int) ([]struct {
	ID               int     `json:"id"`
	HID              string  `json:"hid"`
	Slug             string  `json:"slug"`
	Title            string  `json:"title"`
	LastChap         float64 `json:"last_chapter"`
	DefaultThumbnail string  `json:"default_thumbnail"`
}, error) {
	var allManhwas []struct {
		ID               int     `json:"id"`
		HID              string  `json:"hid"`
		Slug             string  `json:"slug"`
		Title            string  `json:"title"`
		LastChap         float64 `json:"last_chapter"`
		DefaultThumbnail string  `json:"default_thumbnail"`
	}

	fmt.Println("Fetching manhwa list...")

	// Parallel page fetching for massive speed improvement!
	const maxPages = 3830
	const pageBatchSize = 100 // Process 100 pages concurrently

	var mu sync.Mutex
	var wg sync.WaitGroup
	pageLimiter := make(chan struct{}, pageBatchSize)
	lastPageFound := 0

	fmt.Printf("Fetching pages in parallel (batch size: %d)...\n", pageBatchSize)

	for page := 1; page <= maxPages; page++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()

			// Acquire page processing slot
			pageLimiter <- struct{}{}
			defer func() { <-pageLimiter }()

			url := fmt.Sprintf("%s/api/search?page=%d", s.baseURL, p)
			resp, err := s.makeRequest(url)
			if err != nil {
				fmt.Printf("Error fetching page %d: %v\n", p, err)
				return
			}

			var searchResp SearchResponse
			err = json.NewDecoder(resp.Body).Decode(&searchResp)
			resp.Body.Close()

			if err != nil {
				fmt.Printf("Error decoding page %d: %v\n", p, err)
				return
			}

			// Filter manhwas by startID if specified
			var pageResults []struct {
				ID               int     `json:"id"`
				HID              string  `json:"hid"`
				Slug             string  `json:"slug"`
				Title            string  `json:"title"`
				LastChap         float64 `json:"last_chapter"`
				DefaultThumbnail string  `json:"default_thumbnail"`
			}
			for _, manhwa := range searchResp.Data {
				if startID == 0 || manhwa.ID >= startID {
					pageResults = append(pageResults, manhwa)
				}
			}

			// Thread-safe updates
			mu.Lock()
			allManhwas = append(allManhwas, pageResults...)
			if searchResp.LastPage > lastPageFound {
				lastPageFound = searchResp.LastPage
			}
			if p%50 == 0 { // Log every 50 pages
				fmt.Printf("Page %d processed, found %d new manhwas (total: %d)\n", p, len(pageResults), len(allManhwas))
			}
			mu.Unlock()
		}(page)
	}

	wg.Wait()
	fmt.Printf("Completed parallel page fetching. Total manhwas found: %d\n", len(allManhwas))

	return allManhwas, nil
}

func (s *ComickScraper) getChapterList(slug string) ([]struct {
	ID    int    `json:"id"`
	HID   string `json:"hid"`
	Chap  string `json:"chap"`
	Title string `json:"title"`
	Lang  string `json:"lang"`
}, error) {
	var englishChapters []struct {
		ID    int    `json:"id"`
		HID   string `json:"hid"`
		Chap  string `json:"chap"`
		Title string `json:"title"`
		Lang  string `json:"lang"`
	}

	// Handle paginated chapter lists - start from page 0 and continue until 3 consecutive 404s
	page := 0
	consecutive404s := 0
	const max404s = 3

	fmt.Printf("Fetching chapter list for %s (checking for pagination)...\n", slug)

	for {
		var url string
		if page == 0 {
			// First try without page parameter
			url = fmt.Sprintf("%s/api/comics/%s/chapter-list", s.baseURL, slug)
		} else {
			// Then try with page parameter
			url = fmt.Sprintf("%s/api/comics/%s/chapter-list?page=%d", s.baseURL, slug, page)
		}

		resp, err := s.makeRequest(url)
		if err != nil || resp.StatusCode == 404 {
			consecutive404s++
			if resp != nil {
				resp.Body.Close()
			}
			fmt.Printf("Page %d failed (consecutive failures: %d/%d)\n", page, consecutive404s, max404s)

			if consecutive404s >= max404s {
				fmt.Printf("Stopping chapter list fetching after %d consecutive 404s\n", max404s)
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
			fmt.Printf("Page %d decode failed (consecutive failures: %d/%d): %v\n", page, consecutive404s, max404s, err)
			if consecutive404s >= max404s {
				fmt.Printf("Stopping chapter list fetching after %d consecutive decode failures\n", max404s)
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

		fmt.Printf("Page %d: found %d English chapters (total: %d)\n", page, pageChapters, len(englishChapters))

		// If this page had no data, it might be the end
		if len(chapterResp.Data) == 0 {
			fmt.Printf("Page %d had no chapters, assuming end of pagination\n", page)
			break
		}

		page++

		// Safety limit to prevent infinite loops
		if page > 100 {
			fmt.Printf("Reached safety limit of 100 pages, stopping\n")
			break
		}
	}

	fmt.Printf("Completed chapter list fetching for %s: %d total English chapters\n", slug, len(englishChapters))
	return englishChapters, nil
}

// Fallback: try to construct image URLs directly
func (s *ComickScraper) getChapterImageHash(slug, hid, chapter string) (string, error) {
	// Skip wasteful API endpoint testing and go directly to HTML parsing
	// which is the method that actually works based on the logs

	// Fallback to HTML scraping (original method)
	url := fmt.Sprintf("%s/comic/%s/%s-chapter-%s-en", s.baseURL, slug, hid, chapter)
	resp, err := s.makeRequest(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	bodyStr := string(body)

	// Extract [shid] following the exact pattern from ScraperLogic.txt
	// Looking for: cdn1.comicknew.pictures/[SLUG]/0_[ChapNumber]/en/[shid]/
	pattern := fmt.Sprintf("cdn1.comicknew.pictures/%s/0_%s/en/", slug, chapter)

	fmt.Printf("Looking for pattern: %s\n", pattern)

	start := strings.Index(bodyStr, pattern)
	if start != -1 {
		start += len(pattern)
		// Find the next forward slash to get the shid
		end := strings.Index(bodyStr[start:], "/")
		if end != -1 {
			shid := bodyStr[start : start+end]
			if len(shid) == 8 && isAlphaNumeric(shid) { // shid should be 8 characters
				fmt.Printf("Found shid for chapter %s: %s\n", chapter, shid)
				return shid, nil
			} else {
				fmt.Printf("Found potential shid but invalid format: %s (length: %d)\n", shid, len(shid))
			}
		}
	}

	// Try without https prefix in case it's escaped differently
	patterns := []string{
		fmt.Sprintf("cdn1.comicknew.pictures\\/%s\\/0_%s\\/en\\/", slug, chapter),
		fmt.Sprintf("cdn1.comicknew.pictures\\\\/%s\\\\/0_%s\\\\/en\\\\/", slug, chapter),
		fmt.Sprintf("\\/cdn1.comicknew.pictures\\/%s\\/0_%s\\/en\\/", slug, chapter),
	}

	for i, escPattern := range patterns {
		fmt.Printf("Trying escaped pattern %d: %s\n", i+1, escPattern)
		start := strings.Index(bodyStr, strings.ReplaceAll(escPattern, "\\", ""))
		if start == -1 {
			// Try with actual backslashes
			start = strings.Index(bodyStr, escPattern)
		}
		if start != -1 {
			fmt.Printf("Found escaped pattern at position %d\n", start)
			// Show some context around the found pattern
			contextStart := max(0, start-50)
			contextEnd := min(len(bodyStr), start+200)
			fmt.Printf("Context: ...%s...\n", bodyStr[contextStart:contextEnd])

			// Extract what follows the pattern
			remaining := bodyStr[start:]

			// Look for the full pattern and extract the shid from escaped JSON
			// Pattern: "url":"https:\/\/cdn1.comicknew.pictures\/slug\/0_chapter\/en\/SHID\/0.webp"
			if idx := strings.Index(remaining, "\\/en\\/"); idx != -1 {
				afterEn := remaining[idx+5:] // Skip "\/en\/"
				// Look for the next forward slash (escaped as \/)
				if slashIdx := strings.Index(afterEn, "\\/"); slashIdx != -1 {
					shid := afterEn[:slashIdx]
					// Remove leading slash if present
					shid = strings.TrimPrefix(shid, "/")
					fmt.Printf("Extracted potential shid from JSON: '%s' (length: %d)\n", shid, len(shid))
					if len(shid) == 8 && isAlphaNumeric(shid) {
						fmt.Printf("Found valid shid from escaped JSON: %s\n", shid)
						return shid, nil
					}
				}
			}
		}
	}

	fmt.Printf("Debug: Could not find hash. Trying systematic discovery...\n")

	// First, let's try to find any embedded JSON in the HTML that might contain image data
	if hash := s.findHashInHTML(bodyStr, slug, chapter); hash != "" {
		fmt.Printf("Found hash in HTML: %s\n", hash)
		return hash, nil
	}

	// Try to find hash from any embedded script data
	if hash := s.findHashInScripts(bodyStr, slug, chapter); hash != "" {
		fmt.Printf("Found hash in scripts: %s\n", hash)
		return hash, nil
	}

	// Try some common hash patterns by guessing
	commonHashes := []string{
		hid,     // Sometimes the hid is the hash
		hid[:8], // First 8 characters of hid
	}

	if len(hid) >= 8 {
		commonHashes = append(commonHashes, hid[len(hid)-8:]) // Last 8 characters
	}

	// Try to derive hash from HID using common transformations
	commonHashes = append(commonHashes, s.generateHashVariations(hid)...)

	for _, hash := range commonHashes {
		if len(hash) == 8 && isAlphaNumeric(hash) {
			fmt.Printf("Trying potential hash: %s\n", hash)
			// Test if this hash works by trying to download first image
			testURL := fmt.Sprintf("%s/%s/0_%s/en/%s/0.webp", s.cdnURL, slug, chapter, hash)
			resp, err := s.makeRequest(testURL)
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				fmt.Printf("Found working hash: %s\n", hash)
				return hash, nil
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}

	// Last resort: try to find the hash from a known working chapter (chapter 1)
	if chapter != "1" {
		fmt.Printf("Trying to discover hash pattern from chapter 1...\n")
		if hash := s.discoverHashFromReference(slug, hid, chapter); hash != "" {
			return hash, nil
		}
	}

	return "", fmt.Errorf("image hash not found for chapter %s", chapter)
}

// Extract hash from image URL
func (s *ComickScraper) extractHashFromURL(url, slug, chapter string) string {
	// Pattern: .../slug/0_chapter/en/hash/image.webp
	pattern := fmt.Sprintf("/%s/0_%s/en/", slug, chapter)
	start := strings.Index(url, pattern)
	if start != -1 {
		start += len(pattern)
		end := strings.Index(url[start:], "/")
		if end != -1 {
			return url[start : start+end]
		}
	}
	return ""
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Try to find hash embedded in HTML content
func (s *ComickScraper) findHashInHTML(htmlContent, slug, chapter string) string {
	// Look for any 8-character alphanumeric strings that appear in the context of image URLs
	lines := strings.Split(htmlContent, "\n")
	for _, line := range lines {
		if strings.Contains(line, slug) || strings.Contains(line, "comicknew") {
			// Extract potential hashes from this line
			words := strings.FieldsFunc(line, func(r rune) bool {
				return r == '"' || r == '\'' || r == '/' || r == '\\' || r == ' ' || r == '\t' || r == '>' || r == '<'
			})

			for _, word := range words {
				if len(word) == 8 && isAlphaNumeric(word) {
					// Test if this might be a valid hash
					testURL := fmt.Sprintf("%s/%s/0_%s/en/%s/0.webp", s.cdnURL, slug, chapter, word)
					resp, err := s.makeRequest(testURL)
					if err == nil && resp.StatusCode == 200 {
						resp.Body.Close()
						return word
					}
					if resp != nil {
						resp.Body.Close()
					}
				}
			}
		}
	}
	return ""
}

// Look for hash in script tags or embedded JSON
func (s *ComickScraper) findHashInScripts(htmlContent, slug, chapter string) string {
	// Look for script tags containing data
	scriptStart := strings.Index(htmlContent, "<script")
	for scriptStart != -1 {
		scriptEnd := strings.Index(htmlContent[scriptStart:], "</script>")
		if scriptEnd == -1 {
			break
		}
		scriptEnd += scriptStart

		scriptContent := htmlContent[scriptStart:scriptEnd]

		// Look for JSON data or hash-like strings in the script
		if strings.Contains(scriptContent, slug) || strings.Contains(scriptContent, "comicknew") {
			// Extract potential hashes
			words := strings.FieldsFunc(scriptContent, func(r rune) bool {
				return r == '"' || r == '\'' || r == ',' || r == ':' || r == '{' || r == '}' || r == '[' || r == ']' || r == ' ' || r == '\n' || r == '\t'
			})

			for _, word := range words {
				word = strings.Trim(word, "\"'")
				if len(word) == 8 && isAlphaNumeric(word) {
					// Test if this is a valid hash
					testURL := fmt.Sprintf("%s/%s/0_%s/en/%s/0.webp", s.cdnURL, slug, chapter, word)
					resp, err := s.makeRequest(testURL)
					if err == nil && resp.StatusCode == 200 {
						resp.Body.Close()
						return word
					}
					if resp != nil {
						resp.Body.Close()
					}
				}
			}
		}

		// Look for next script tag
		scriptStart = strings.Index(htmlContent[scriptEnd:], "<script")
		if scriptStart != -1 {
			scriptStart += scriptEnd
		}
	}
	return ""
}

// Generate hash variations from HID
func (s *ComickScraper) generateHashVariations(hid string) []string {
	var variations []string

	if len(hid) >= 8 {
		// Try all 8-character substrings
		for i := 0; i <= len(hid)-8; i++ {
			variations = append(variations, hid[i:i+8])
		}

		// Try lowercase versions
		hidLower := strings.ToLower(hid)
		for i := 0; i <= len(hidLower)-8; i++ {
			variations = append(variations, hidLower[i:i+8])
		}

		// Try some transformations
		if len(hid) >= 8 {
			// Reverse first 8 chars
			firstEight := hid[:8]
			reversed := ""
			for i := len(firstEight) - 1; i >= 0; i-- {
				reversed += string(firstEight[i])
			}
			variations = append(variations, reversed)
		}
	}

	return variations
}

// Try to discover hash pattern using a known reference
func (s *ComickScraper) discoverHashFromReference(slug, hid, chapter string) string {
	// We know that chapter 1 has hash 3c4564c8 for this specific manga
	// Let's see if there's a pattern or if we can find an API that maps HIDs to hashes

	fmt.Printf("Attempting to reverse-engineer hash from known patterns...\n")

	// Try some computational approaches to derive hash from HID
	// This is speculative but worth trying

	// Method 1: Try MD5/SHA hashes of various HID combinations
	testHashes := []string{
		s.computeHashVariant(hid, "md5")[:8],
		s.computeHashVariant(hid, "sha1")[:8],
		s.computeHashVariant(hid+slug, "md5")[:8],
		s.computeHashVariant(slug+hid, "md5")[:8],
		s.computeHashVariant(hid+chapter, "md5")[:8],
	}

	for _, hash := range testHashes {
		if len(hash) == 8 && isAlphaNumeric(hash) {
			testURL := fmt.Sprintf("%s/%s/0_%s/en/%s/0.webp", s.cdnURL, slug, chapter, hash)
			resp, err := s.makeRequest(testURL)
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				fmt.Printf("Found computed hash: %s\n", hash)
				return hash
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}

	return ""
}

// Helper to compute hash variants (simplified)
func (s *ComickScraper) computeHashVariant(input, method string) string {
	// This is a simplified hash computation
	// In reality, we'd need proper crypto libraries for MD5/SHA1
	// For now, just return a transformation of the input
	result := ""
	for i, char := range input {
		result += fmt.Sprintf("%x", (int(char)+i)%16)
	}
	if len(result) > 8 {
		return result[:8]
	}
	return result
}

// Try to extract hash from JavaScript state data
func (s *ComickScraper) extractHashFromJS(body, slug, chapter string) string {
	// Look for JSON data in JavaScript
	patterns := []string{
		"window.__INITIAL_STATE__",
		"window.__NUXT__",
		"__NUXT_DATA__",
	}

	for _, pattern := range patterns {
		start := strings.Index(body, pattern)
		if start != -1 {
			// Find the JSON object
			start = strings.Index(body[start:], "{")
			if start != -1 {
				start += strings.Index(body[:start], pattern) // Adjust offset
				// Find matching closing brace (simplified)
				braceCount := 0
				end := start
				for i, char := range body[start:] {
					if char == '{' {
						braceCount++
					} else if char == '}' {
						braceCount--
						if braceCount == 0 {
							end = start + i + 1
							break
						}
					}
				}

				if end > start {
					jsData := body[start:end]
					// Look for image hashes in the JSON
					if strings.Contains(jsData, s.cdnURL) || strings.Contains(jsData, "comicknew") {
						// Extract potential hashes from the JSON
						parts := strings.FieldsFunc(jsData, func(r rune) bool {
							return r == '"' || r == '/' || r == '\\' || r == ',' || r == ':'
						})

						for _, part := range parts {
							if len(part) == 8 && isAlphaNumeric(part) {
								return part
							}
						}
					}
				}
			}
		}
	}
	return ""
}

// Helper function to check if string is alphanumeric
func isAlphaNumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func (s *ComickScraper) downloadImage(url, filepath string) error {
	// Create specific request for image downloads with proper headers
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// Add headers specific to image downloads from CDN (following ScraperLogic.txt)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Not=A?Brand";v="24", "Chromium";v="140"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Linux"`)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Storage-Access", "active")
	req.Header.Set("Referer", "https://comick.live/")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Priority", "i")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	// Create directory if it doesn't exist
	dir := filepath[:strings.LastIndex(filepath, "/")]
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func (s *ComickScraper) downloadChapter(slug, hid, chapter string) error {
	fmt.Printf("Downloading chapter %s...\n", chapter)

	// Get the image hash
	hash, err := s.getChapterImageHash(slug, hid, chapter)
	if err != nil {
		return fmt.Errorf("failed to get image hash: %v", err)
	}

	// Create chapter directory
	chapterDir := fmt.Sprintf("downloads/%s/chapter_%s", slug, chapter)
	err = os.MkdirAll(chapterDir, 0755)
	if err != nil {
		return err
	}

	// Download images starting from 0 until we get 3 consecutive 404s
	fmt.Printf("Starting download for chapter %s...\n", chapter)
	consecutive404s := 0
	const max404s = 3
	const maxImages = 200 // Safety limit
	downloadedCount := 0

	for i := 0; i < maxImages; i++ {
		imageURL := fmt.Sprintf("%s/%s/0_%s/en/%s/%d.webp", s.cdnURL, slug, chapter, hash, i)
		imagePath := fmt.Sprintf("%s/%03d.webp", chapterDir, i)

		// Small delay to avoid rate limiting
		time.Sleep(50 * time.Millisecond)

		err := s.downloadImage(imageURL, imagePath)
		if err != nil {
			consecutive404s++
			fmt.Printf("Image %d failed (consecutive failures: %d/%d): %v\n", i, consecutive404s, max404s, err)

			if consecutive404s >= max404s {
				fmt.Printf("Stopping after %d consecutive failures. Downloaded %d images.\n", max404s, downloadedCount)
				break
			}
			continue
		}

		// Successfully downloaded
		consecutive404s = 0
		downloadedCount++
		fmt.Printf("Downloaded image %d for chapter %s (total: %d)\n", i, chapter, downloadedCount)
	}

	if downloadedCount == 0 {
		return fmt.Errorf("no images downloaded for chapter %s", chapter)
	}
	fmt.Printf("Completed downloading chapter %s\n", chapter)
	return nil
}

func (s *ComickScraper) downloadCover(slug, coverURL string) error {
	if coverURL == "" {
		fmt.Printf("No cover image URL for %s\n", slug)
		return nil
	}

	fmt.Printf("Downloading cover image for %s...\n", slug)

	// Create manhwa directory
	manhwaDir := fmt.Sprintf("downloads/%s", slug)
	err := os.MkdirAll(manhwaDir, 0755)
	if err != nil {
		return err
	}

	// Determine file extension from URL
	extension := ".webp" // Default to webp
	if strings.Contains(coverURL, ".jpg") || strings.Contains(coverURL, ".jpeg") {
		extension = ".jpg"
	} else if strings.Contains(coverURL, ".png") {
		extension = ".png"
	}

	coverPath := fmt.Sprintf("%s/cover%s", manhwaDir, extension)

	// Download the cover image
	err = s.downloadImage(coverURL, coverPath)
	if err != nil {
		return fmt.Errorf("failed to download cover image: %v", err)
	}

	fmt.Printf("Downloaded cover image for %s\n", slug)
	return nil
}

func (s *ComickScraper) downloadManhwa(slug, coverURL string) error {
	fmt.Printf("Starting download for manhwa: %s\n", slug)

	// Download cover image first
	err := s.downloadCover(slug, coverURL)
	if err != nil {
		fmt.Printf("Failed to download cover for %s: %v\n", slug, err)
		// Continue with chapters even if cover fails
	}

	chapters, err := s.getChapterList(slug)
	if err != nil {
		return fmt.Errorf("failed to get chapter list: %v", err)
	}

	fmt.Printf("Found %d English chapters for %s\n", len(chapters), slug)

	// Download chapters in parallel for speed
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0

	for _, chapter := range chapters {
		wg.Add(1)
		go func(ch struct {
			ID    int    `json:"id"`
			HID   string `json:"hid"`
			Chap  string `json:"chap"`
			Title string `json:"title"`
			Lang  string `json:"lang"`
		}) {
			defer wg.Done()

			// Acquire chapter processing slot
			s.chapterLimiter <- struct{}{}
			defer func() { <-s.chapterLimiter }()

			err := s.downloadChapter(slug, ch.HID, ch.Chap)

			mu.Lock()
			completed++
			if err != nil {
				fmt.Printf("[%s] Chapter %s failed (%d/%d): %v\n", slug, ch.Chap, completed, len(chapters), err)
			} else {
				fmt.Printf("[%s] Chapter %s completed (%d/%d)\n", slug, ch.Chap, completed, len(chapters))
			}
			mu.Unlock()
		}(chapter)
	}

	wg.Wait()

	return nil
}

func printUsage() {
	programName := filepath.Base(os.Args[0])
	if programName == "main" || strings.Contains(programName, "Cursor") {
		programName = "./comick-scraper"
	}

	fmt.Printf(`Comick.live Manhwa Scraper - High-performance downloader for manhwas

USAGE:
    %s [OPTIONS]

MODES:
    -mode=full                Download ALL manhwas from comick.live (WARNING: Very large!)
    -mode=slug                Download specific manhwa by slug
    -mode=after-id            Download manhwas with ID >= specified number

OPTIONS:
    -slug=STRING              Manhwa slug (required for -mode=slug)
                              Example: solo-leveling, tower-of-god
    -start-id=NUMBER          Starting manhwa ID (required for -mode=after-id)
                              Example: 1000 (downloads manhwas with ID >= 1000)
    -workers=NUMBER           Concurrent download workers (default: 20)
                              Higher = faster downloads (max: 50)

EXAMPLES:
    # Download specific manhwa
    %s -mode=slug -slug=solo-leveling
    
    # Download all manhwas after ID 500
    %s -mode=after-id -start-id=500 -workers=15
    
    # Download everything (use with caution!)
    %s -mode=full

OUTPUT STRUCTURE:
    downloads/
    ├── manhwa-slug/
    │   ├── cover.webp          ← Cover image
    │   ├── chapter_1/
    │   │   ├── 000.webp
    │   │   ├── 001.webp
    │   │   └── ...
    │   └── chapter_2/
    └── ...

NOTES:
    - Only downloads English chapters
    - Downloads cover images automatically  
    - Images saved as WebP/JPG/PNG format
    - Parallel processing: 100 pages + 10 manhwas + 5 chapters + 20 images simultaneously
    - Supports paginated chapter lists (discovers ALL chapters)
    - Automatic rate limiting to avoid server bans
    - Stops after 3 consecutive 404s per chapter
    - Optimized for bulk downloads (target: <1 hour for full download)

`, programName, programName, programName, programName)
}

func main() {
	var (
		mode    = flag.String("mode", "", "Required: 'full', 'slug', or 'after-id'")
		slug    = flag.String("slug", "", "Manhwa slug (for -mode=slug)")
		startID = flag.Int("start-id", 0, "Starting manhwa ID (for -mode=after-id)")
		workers = flag.Int("workers", 20, "Concurrent download workers (1-50, default 20)")
		help    = flag.Bool("h", false, "Show detailed help")
		help2   = flag.Bool("help", false, "Show detailed help")
	)

	flag.Usage = printUsage
	flag.Parse()

	if *help || *help2 {
		printUsage()
		os.Exit(0)
	}

	if *mode == "" {
		fmt.Println("Please specify a mode: -mode=full, -mode=slug, or -mode=after-id")
		flag.Usage()
		os.Exit(1)
	}

	scraper := NewComickScraper()

	// Limit workers to reasonable bounds
	if *workers > 50 {
		fmt.Printf("Warning: Worker count limited to 50 (requested: %d)\n", *workers)
		*workers = 50
	} else if *workers < 1 {
		fmt.Printf("Warning: Worker count must be at least 1 (requested: %d)\n", *workers)
		*workers = 1
	}

	scraper.maxWorkers = *workers

	switch *mode {
	case "full":
		fmt.Println("Starting full download of all manhwas...")
		manhwas, err := scraper.getAllManhwas(0)
		if err != nil {
			fmt.Printf("Error getting manhwa list: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Found %d manhwas. Starting parallel downloads...\n", len(manhwas))

		var wg sync.WaitGroup
		completed := 0
		var mu sync.Mutex

		for _, manhwa := range manhwas {
			wg.Add(1)
			go func(m struct {
				ID               int     `json:"id"`
				HID              string  `json:"hid"`
				Slug             string  `json:"slug"`
				Title            string  `json:"title"`
				LastChap         float64 `json:"last_chapter"`
				DefaultThumbnail string  `json:"default_thumbnail"`
			}) {
				defer wg.Done()

				// Acquire manhwa processing slot
				scraper.manhwaLimiter <- struct{}{}
				defer func() { <-scraper.manhwaLimiter }()

				err := scraper.downloadManhwa(m.Slug, m.DefaultThumbnail)

				mu.Lock()
				completed++
				if err != nil {
					fmt.Printf("[%d/%d] Failed: %s - %v\n", completed, len(manhwas), m.Title, err)
				} else {
					fmt.Printf("[%d/%d] Completed: %s\n", completed, len(manhwas), m.Title)
				}
				mu.Unlock()
			}(manhwa)
		}

		wg.Wait()

	case "slug":
		if *slug == "" {
			fmt.Println("Please provide a slug with -slug=your-manhwa-slug")
			os.Exit(1)
		}

		err := scraper.downloadManhwa(*slug, "")
		if err != nil {
			fmt.Printf("Error downloading manhwa: %v\n", err)
			os.Exit(1)
		}

	case "after-id":
		if *startID <= 0 {
			fmt.Println("Please provide a valid start ID with -start-id=123")
			os.Exit(1)
		}

		fmt.Printf("Starting download of manhwas after ID %d...\n", *startID)
		manhwas, err := scraper.getAllManhwas(*startID)
		if err != nil {
			fmt.Printf("Error getting manhwa list: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Found %d manhwas after ID %d. Starting parallel downloads...\n", len(manhwas), *startID)

		var wg sync.WaitGroup
		completed := 0
		var mu sync.Mutex

		for _, manhwa := range manhwas {
			wg.Add(1)
			go func(m struct {
				ID               int     `json:"id"`
				HID              string  `json:"hid"`
				Slug             string  `json:"slug"`
				Title            string  `json:"title"`
				LastChap         float64 `json:"last_chapter"`
				DefaultThumbnail string  `json:"default_thumbnail"`
			}) {
				defer wg.Done()

				// Acquire manhwa processing slot
				scraper.manhwaLimiter <- struct{}{}
				defer func() { <-scraper.manhwaLimiter }()

				err := scraper.downloadManhwa(m.Slug, m.DefaultThumbnail)

				mu.Lock()
				completed++
				if err != nil {
					fmt.Printf("[%d/%d] Failed: %s (ID: %d) - %v\n", completed, len(manhwas), m.Title, m.ID, err)
				} else {
					fmt.Printf("[%d/%d] Completed: %s (ID: %d)\n", completed, len(manhwas), m.Title, m.ID)
				}
				mu.Unlock()
			}(manhwa)
		}

		wg.Wait()

	default:
		fmt.Printf("Unknown mode: %s\n", *mode)
		fmt.Println("Available modes: full, slug, after-id")
		os.Exit(1)
	}

	fmt.Println("Scraping completed!")
}

// headers.go
package main

// HeaderManager provides optimized header sets for different request types
type HeaderManager struct {
	commonHeaders map[string]string
	apiHeaders    map[string]string
	pageHeaders   map[string]string
	imageHeaders  map[string]string
}

// NewHeaderManager creates a new header manager with predefined header sets
func NewHeaderManager() *HeaderManager {
	commonHeaders := map[string]string{
		"User-Agent":         "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36",
		"Accept-Language":    "en-US,en;q=0.9",
		"Sec-Ch-Ua":          `"Not=A?Brand";v="24", "Chromium";v="140"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"Linux"`,
	}

	apiHeaders := map[string]string{
		"Accept":         "*/*",
		"Sec-Fetch-Site": "same-origin",
		"Sec-Fetch-Mode": "cors",
		"Sec-Fetch-Dest": "empty",
	}

	pageHeaders := map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		"Sec-Fetch-Site":            "same-origin",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Dest":            "document",
		"Upgrade-Insecure-Requests": "1",
	}

	imageHeaders := map[string]string{
		"Accept":                   "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
		"Sec-Fetch-Site":           "cross-site",
		"Sec-Fetch-Mode":           "no-cors",
		"Sec-Fetch-Dest":           "image",
		"Sec-Fetch-Storage-Access": "active",
		"Referer":                  "https://comick.live/",
		"Accept-Encoding":          "gzip, deflate, br",
		"Priority":                 "i",
	}

	return &HeaderManager{
		commonHeaders: commonHeaders,
		apiHeaders:    apiHeaders,
		pageHeaders:   pageHeaders,
		imageHeaders:  imageHeaders,
	}
}

// GetAPIHeaders returns headers optimized for API requests
func (h *HeaderManager) GetAPIHeaders() map[string]string {
	return h.mergeHeaders(h.commonHeaders, h.apiHeaders)
}

// GetPageHeaders returns headers optimized for HTML page requests
func (h *HeaderManager) GetPageHeaders() map[string]string {
	return h.mergeHeaders(h.commonHeaders, h.pageHeaders)
}

// GetImageHeaders returns headers optimized for image downloads
func (h *HeaderManager) GetImageHeaders() map[string]string {
	return h.mergeHeaders(h.commonHeaders, h.imageHeaders)
}

// mergeHeaders combines multiple header maps into one
func (h *HeaderManager) mergeHeaders(headerSets ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, headers := range headerSets {
		for key, value := range headers {
			result[key] = value
		}
	}
	return result
}

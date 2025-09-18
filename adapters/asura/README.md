# AsuraComic.net Adapter

A specialized adapter for scraping manga/manhwa from asuracomic.net using HTML parsing and pattern matching.

## 🌟 Features

- **HTML parsing** - Direct scraping from asuracomic.net web pages
- **Sequential chapter discovery** - Systematic enumeration of chapters
- **CDN optimization** - Direct access to gg.asuracomic.net image CDN
- **Smart termination** - Stops after consecutive 404s
- **Cover extraction** - Automatic cover image downloading
- **Robust error handling** - Graceful failure recovery

## 🏗️ Architecture

The Asura adapter implements the `SiteScraper` interface with these key components:

### Core Structure
```go
type AsuraAdapter struct {
    config      Config
    fetcher     Fetcher
    baseURL     string      // https://asuracomic.net
    cdnURL      string      // https://gg.asuracomic.net
    seriesPool  *WorkerPool // Series-level concurrency
    chapterPool *WorkerPool // Chapter-level concurrency
    imagePool   *WorkerPool // Image download concurrency
}
```

### URL Patterns
- **Series List**: `/series?page={page}` - Series discovery
- **Series Page**: `/series/{slug}` - Cover extraction and metadata
- **Chapter Page**: `/series/{slug}/chapter/{number}` - Image path discovery
- **Images**: `/storage/media/{path}/conversions/{num}-optimized.webp`

## 📊 Performance Characteristics

| Operation | Concurrency | Rate Limit | Notes |
|-----------|-------------|------------|-------|
| Series Discovery | 20 parallel pages | Built-in | HTML parsing |
| Series Processing | 10 concurrent | Built-in | Balanced load |
| Chapter Processing | 5 per series | Built-in | Server-friendly |
| Image Downloads | User configurable | 50ms delay | CDN optimized |

## 🔧 Configuration Options

The adapter supports these configuration parameters:

```go
config := Config{
    MaxSeriesWorkers:  10,    // Concurrent series processing
    MaxChapterWorkers: 5,     // Chapters per series
    MaxImageWorkers:   20-50, // Image download workers
    HTTPTimeout:       15s,   // Request timeout
    MaxRetries:        3,     // Retry attempts
    RetryDelay:        1s,    // Base retry delay
}
```

## 🚀 Usage Examples

### Download Specific Series
```bash
./scraper -site=asura -mode=slug -slug=reaper-of-the-drifting-moon-4e28152d
./scraper -site=asura -mode=slug -slug=martial-peak-6c23a1e2 -workers=25
```

### Full Site Download
```bash
# Download all series (use with caution!)
./scraper -site=asura -mode=full -workers=30
```

## 📁 Output Structure

```
downloads/
├── reaper-of-the-drifting-moon/
│   ├── cover.webp           # Series cover image
│   ├── chapter_1/
│   │   ├── 000.webp
│   │   ├── 001.webp
│   │   └── ...
│   ├── chapter_2/
│   └── ...
└── ...
```

## 🔍 HTML Parsing Strategy

The adapter uses sophisticated HTML parsing techniques:

### 1. Series Discovery
Extracts series links from paginated lists:
```regex
<a href="series/([^"]+)">
```

### 2. Cover Image Extraction
Finds high-quality cover images:
```regex
/media/\d+/[^"'\s]+\.webp
```
Filters out thumbnails and optimized versions.

### 3. Image Path Discovery
Extracts media paths from chapter pages:
```regex
gg\.asuracomic\.net/storage/media/(\d+)/conversions/0\d+-optimized\.webp
```

## 🎯 Series Slug Format

AsuraComic uses a specific slug format:
- **Pattern**: `{title}-{hid}`
- **Example**: `reaper-of-the-drifting-moon-4e28152d`
- **Components**: 
  - Title: `reaper-of-the-drifting-moon`
  - HID: `4e28152d` (8-character hash)

## 🔬 Technical Details

### Chapter Numbering
- **Internal**: 0-based (chapter/0, chapter/1, ...)
- **Output**: 1-based (chapter_1/, chapter_2/, ...)
- **Images**: 0-based (000.webp, 001.webp, ...)

### Image URL Construction
```
https://gg.asuracomic.net/storage/media/{media_id}/conversions/{num:02d}-optimized.webp
```

### Termination Logic
- Stops after 3 consecutive 404s on chapters
- Stops after 3 consecutive 404s on images
- Safety limits: 500 chapters max, 200 images per chapter

### Concurrency Model
- **Series Discovery**: Parallel page fetching (up to 20 pages)
- **Series Processing**: 10 series processed concurrently
- **Chapter Processing**: 5 chapters per series with batch waiting
- **Image Downloads**: Configurable workers with rate limiting

## 🛠️ Error Handling

### Graceful Failures
- **404 Detection**: Smart consecutive failure tracking
- **Rate Limiting**: Built-in delays and respectful request patterns
- **Retry Logic**: Exponential backoff for transient errors
- **Logging**: Comprehensive debug information

### Recovery Strategies
- Continues processing other series if one fails
- Handles missing covers gracefully
- Recovers from temporary network issues
- Provides detailed error context

## 🐛 Troubleshooting

### Common Issues

**Series not found:**
```bash
# Verify the slug format includes the HID
./scraper -site=asura -mode=slug -slug=full-series-name-with-hid -log=debug
```

**Image path extraction failures:**
```bash
# Enable debug logging to see HTML parsing
./scraper -site=asura -mode=slug -slug=series-name -log=debug
```

**Rate limiting:**
```bash
# Reduce concurrent workers
./scraper -site=asura -mode=slug -slug=series-name -workers=15
```

### Finding Series Slugs

1. Visit asuracomic.net
2. Navigate to any series page
3. Copy the slug from the URL:
   - URL: `https://asuracomic.net/series/reaper-of-the-drifting-moon-4e28152d`
   - Slug: `reaper-of-the-drifting-moon-4e28152d`

## 📈 Performance Benchmarks

Based on testing with various configurations:

| Series Count | Workers | Avg Time | Notes |
|-------------|---------|----------|-------|
| 1 series | 25 | 3-8 min | Depends on chapter count |
| 10 series | 30 | 20-35 min | Typical manhwa length |
| All series (~200) | 25-30 | 2-4 hours | Full site download |

## 🔧 Site-Specific Optimizations

### Header Management
- Uses appropriate browser headers for HTML requests
- Includes referrer for image downloads
- Handles security headers properly

### Rate Limiting
- 50ms delays between image downloads
- Batch processing for chapter discovery
- Server-friendly request patterns

### Error Recovery
- Handles AsuraComic's specific error pages
- Recovers from temporary server issues
- Graceful handling of missing content

## 🔮 Future Enhancements

- [ ] **Chapter range selection** - Download specific chapter ranges
- [ ] **Quality options** - Support for different image qualities
- [ ] **Metadata extraction** - Series info, tags, status
- [ ] **Resume capability** - Continue interrupted downloads
- [ ] **Search functionality** - Find series by title/genre

## 🚨 Important Notes

### Site Structure
- AsuraComic has fewer total series (~200) compared to other sites
- Series are organized in ~20 pages maximum
- Chapter numbering starts from 0 internally

### Legal Considerations
- Educational and personal use only
- Respect AsuraComic's terms of service
- Built-in rate limiting prevents server overload
- Not intended for commercial use

## 📝 Development Notes

### Testing
```bash
# Test with a short series first
./scraper -site=asura -mode=slug -slug=short-series-name -workers=20 -log=debug

# Monitor for HTML structure changes
./scraper -site=asura -mode=slug -slug=test-series -log=debug | grep "pattern"
```

### Debugging HTML Changes
If the site structure changes, enable debug logging to see:
- HTML parsing results
- Regex pattern matches
- URL construction process

---

**Built for the manhwa community with ❤️**

*Combining HTML parsing expertise with robust error handling*


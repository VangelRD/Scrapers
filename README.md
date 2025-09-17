# Comick.so Manhwa Scraper

A high-performance Go scraper for downloading manhwas from comick.so. Built for speed with concurrent downloads and efficient API handling.

## Features

- **3 Scraping Modes**: Full download, specific slug, or download after a specific manhwa ID
- **Cover Images**: Automatically downloads cover images for each manhwa
- **Concurrent Downloads**: Configurable worker pool for fast image downloading
- **Rate Limiting**: Built-in rate limiting to respect the server
- **English Only**: Automatically filters for English chapters
- **Organized Storage**: Downloads are organized by manhwa slug and chapter number
- **Robust Error Handling**: Continues downloading even if individual chapters fail

## Installation

1. Make sure you have Go installed (version 1.21 or higher)
2. Clone or download this repository
3. Navigate to the project directory
4. Build the scraper:

```bash
go build -o comick-scraper main.go
```

## Usage

The scraper supports three modes:

### 1. Full Download (Download All Manhwas)

Downloads all available manhwas and their chapters from comick.so:

```bash
./comick-scraper -mode=full
```

**Warning**: This will download thousands of manhwas and can take days to complete. Make sure you have sufficient storage space.

### 2. Download Specific Manhwa by Slug

Download all chapters for a specific manhwa using its slug:

```bash
./comick-scraper -mode=slug -slug=the-mad-dog-of-the-duke-s-estate
```

To find a manhwa's slug, check its URL on comick.so. For example:
- URL: `https://comick.so/comic/the-mad-dog-of-the-duke-s-estate`
- Slug: `the-mad-dog-of-the-duke-s-estate`

### 3. Download Manhwas After Specific ID

Download all manhwas that have an ID greater than or equal to the specified ID:

```bash
./comick-scraper -mode=after-id -start-id=1000
```

This is useful for resuming downloads or only getting newer manhwas.

## Configuration Options

### Concurrent Workers

Control the number of concurrent download workers (default: 10):

```bash
./comick-scraper -mode=slug -slug=your-manhwa -workers=20
```

**Note**: Higher worker counts may download faster but could get your IP rate-limited or banned. Use responsibly.

## Download Structure

Downloads are organized in the following structure:

```
downloads/
├── manhwa-slug-1/
│   ├── cover.webp          ← Cover image
│   ├── chapter_1/
│   │   ├── 000.webp
│   │   ├── 001.webp
│   │   └── ...
│   ├── chapter_2/
│   │   ├── 000.webp
│   │   ├── 001.webp
│   │   └── ...
│   └── ...
├── manhwa-slug-2/
│   ├── cover.webp
│   └── ...
└── ...
```

## Examples

### Download a single manhwa:
```bash
./comick-scraper -mode=slug -slug=solo-leveling
```

### Download all manhwas with high concurrency:
```bash
./comick-scraper -mode=full -workers=15
```

### Resume downloading from manhwa ID 500:
```bash
./comick-scraper -mode=after-id -start-id=500 -workers=8
```

## Technical Details

The scraper follows the API pattern discovered from comick.so:

1. **Get Manhwa List**: Uses `/api/search?page=N` to get all available manhwas
2. **Get Chapters**: Uses `/api/comics/[SLUG]/chapter-list` to get chapter information
3. **Get Image Hash**: Fetches the chapter page to extract the image hash
4. **Download Images**: Downloads images from CDN using pattern: `cdn1.comicknew.pictures/[SLUG]/0_[CHAPTER]/en/[HASH]/[N].webp`

## Rate Limiting

The scraper includes built-in rate limiting:
- Maximum 5 concurrent API requests
- 100ms delay between API calls
- Configurable download workers (default: 10)

## Error Handling

- Failed downloads are logged but don't stop the scraper
- 404 errors are used to detect the end of image sequences
- Network timeouts are handled gracefully
- Invalid responses are skipped with error messages

## Performance

- Written in Go for maximum performance
- Concurrent downloads with goroutines
- Memory-efficient streaming downloads
- Minimal API calls required per chapter

## Legal Notice

This tool is for educational purposes only. Please respect comick.so's terms of service and use responsibly. Don't overwhelm their servers with too many concurrent requests.

## Troubleshooting

### Common Issues:

1. **"Permission denied" errors**: Make sure the scraper has write permissions in the current directory
2. **"Too many open files" error**: Reduce the number of workers with `-workers=5`
3. **Network timeouts**: The scraper will retry automatically, but you may need to reduce concurrency
4. **Disk space**: Each manhwa can be several hundred MB to GB in size

### Finding Manhwa Slugs:

Visit comick.so and navigate to any manhwa. The slug is the last part of the URL:
- `https://comick.so/comic/tower-of-god` → slug: `tower-of-god`
- `https://comick.so/comic/one-piece` → slug: `one-piece`

## Contributing

Feel free to submit issues and pull requests to improve the scraper!

# Universal Scraper Framework (Go)

A modular, high-performance scraping framework in Go. This repository provides the core interfaces, concurrency primitives, and utilities to build and run site-specific adapters safely and responsibly.

## 🏗️ Project Structure

```
/
├── 🔧 Core Files
│   ├── main.go              # CLI interface and configuration
│   ├── interfaces.go        # Adapter interfaces (SiteScraper, Fetcher)
│   ├── types.go             # Config types and WorkerPool
│   ├── utils.go             # Logging, file/io helpers, headers
│   ├── fetcher.go           # HTTP client abstraction
│   ├── build.sh             # Build script
│   ├── go.mod               # Go module definition
│   └── README.md            # This file
└── 📦 Generated
    └── scraper              # Compiled binary (ignored in VCS)
```

## 🚀 Quick Start

### Build
```bash
chmod +x build.sh
./build.sh
# or
go build -o scraper *.go
```

### CLI Overview
```bash
./scraper -h
```
Flags are site-agnostic. Concrete sites are added by implementing adapters and registering them in `main.go`.

- `-site=STRING`          Identifier for a registered adapter (required)
- `-mode=STRING`          `full` or `slug` (adapter-defined semantics)
- `-slug=STRING`          Series identifier for slug mode
- `-workers=NUMBER`       Concurrent worker limit (1-50)
- `-log=STRING`           Log level: debug, info, warn, error

## 🎯 Design Goals

- **Safety first**: Respect Terms of Service, `robots.txt`, and rate limits.
- **Performance**: Streamed downloads, bounded concurrency, targeted retries.
- **Modularity**: Clean interfaces and swappable components.
- **Maintainability**: Clear separation of discovery, parsing, and downloading.

## 🔌 Adapter Architecture

Adapters implement `SiteScraper` from `interfaces.go`:

```go
// interfaces.go
// SiteScraper defines the interface that all site adapters must implement
type SiteScraper interface {
    // Core download methods
    DownloadAll() error
    DownloadBySlug(slug string) error

    // Site identification
    GetSiteName() string
}
```

Helper abstractions are provided:
- `Fetcher` interface to encapsulate HTTP requests (see `fetcher.go`)
- `Config` for timeouts, concurrency, retries (see `types.go`)
- `WorkerPool` for bounded concurrency (see `types.go`)
- Utility helpers for logging, dirs, and file downloads (see `utils.go`)

## 🧩 How to Build an Adapter (Detailed Guide)

This is a pragmatic, repeatable process to create a robust, polite, and maintainable adapter.

### 1) Create your adapter file
```bash
# Example
touch newsite_adapter.go
```

```go
// newsite_adapter.go
package main

type NewSiteAdapter struct {
    config      Config
    fetcher     Fetcher
    baseURL     string
    seriesPool  *WorkerPool
    chapterPool *WorkerPool
    imagePool   *WorkerPool
}

func NewNewSiteAdapter(config Config) *NewSiteAdapter {
    return &NewSiteAdapter{
        config:      config,
        fetcher:     NewHTTPFetcher(config.HTTPTimeout),
        baseURL:     "https://example.com",
        seriesPool:  NewWorkerPool(config.MaxSeriesWorkers),
        chapterPool: NewWorkerPool(config.MaxChapterWorkers),
        imagePool:   NewWorkerPool(config.MaxImageWorkers),
    }
}

func (n *NewSiteAdapter) GetSiteName() string { return "newsite" }
func (n *NewSiteAdapter) DownloadAll() error  { /* implement */ return nil }
func (n *NewSiteAdapter) DownloadBySlug(slug string) error { /* implement */ return nil }
```

### 2) Wire your adapter into `main.go`

Adapters live in the same Go package (`package main`), so no imports are needed. Register your adapter in two places:

- **Single-site mode switch** (used when `-site=<yoursite>`):

```go
// main.go (excerpt)
var scraper SiteScraper

switch *site {
case "newsite":
    scraper = NewNewSiteAdapter(config)
    log.Println("Initialized newsite scraper")
// add more cases for other sites
default:
    log.Fatalf("Unknown site: %s", *site)
}

// Execute based on -mode
globalErr := error(nil)
switch *mode {
case "full":
    globalErr = scraper.DownloadAll()
case "slug":
    if *slug == "" {
        log.Fatal("Please provide a slug with -slug=series-slug")
    }
    globalErr = scraper.DownloadBySlug(*slug)
default:
    log.Fatalf("Unknown mode: %s", *mode)
}
if globalErr != nil { log.Fatalf("Scraping failed: %v", globalErr) }
```

- **Optional multi-site orchestration** (if you support a mode that runs all adapters concurrently):

```go
// main.go (optional multi-site function)
adapters := map[string]SiteScraper{
    "newsite": NewNewSiteAdapter(config),
    // add other adapters here
}
// iterate this map concurrently and run the same mode/slug against each
```

- **CLI help text**: Update the usage banner in `printUsage()` to list your adapter key (e.g., `newsite`).

```go
// printUsage() (excerpt)
fmt.Printf(`SUPPORTED SITES:
    - newsite (newsite)
`)
```

- **Validation**: If your adapter supports only a subset of modes, validate early and print a clear error.

```go
if *mode == "slug" && *slug == "" {
    log.Fatal("-mode=slug requires -slug=<id>")
}
```

### 3) HTTP policy: headers and ethics
- Use `Fetcher` to issue requests.
- Use realistic `User-Agent` and `Accept-Language` via `GetCommonHeaders()`.
- Avoid spoofing browser-only `Sec-*` and `Sec-CH-UA` headers.
- Honor site policies: Terms of Service and `robots.txt`.
- Never bypass authentication, CAPTCHAs, paywalls, or protection measures.

#### Minimal header example
```go
headers := GetCommonHeaders()
headers["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
```

### 4) robots.txt and allow-listing
Fetch and evaluate `robots.txt` before crawling. If disallowed, do not crawl.

```go
func isAllowedByRobots(baseURL, path string, fetcher Fetcher) bool {
    robotsURL := strings.TrimRight(baseURL, "/") + "/robots.txt"
    resp, err := fetcher.Get(robotsURL, GetCommonHeaders())
    if err != nil || resp.StatusCode != 200 {
        if resp != nil { resp.Body.Close() }
        return true // be conservative with false negatives in demos; consider default deny
    }
    body, _ := io.ReadAll(resp.Body)
    resp.Body.Close()
    // Minimal check: disallow exact path lines. For production, use a proper parser.
    for _, line := range strings.Split(string(body), "\n") {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(strings.ToLower(line), "disallow:") {
            rule := strings.TrimSpace(strings.TrimPrefix(line, "Disallow:"))
            if rule != "" && strings.HasPrefix(path, rule) {
                return false
            }
        }
    }
    return true
}
```

### 5) Concurrency and global rate limiting
Use `WorkerPool` to bound concurrency per resource type and a global limiter for politeness.

```go
// Simple token bucket using a ticker
var globalTokens = make(chan struct{}, 4) // up to 4 requests in flight
go func() {
    ticker := time.NewTicker(250 * time.Millisecond) // ~4 req/sec
    for range ticker.C {
        select { case globalTokens <- struct{}{}: default: }
    }
}()

func acquireGlobal() func() {
    <-globalTokens
    return func() {}
}
```

Use it around each outbound request.

### 6) Discovery strategies (choose what fits the site)
- **Static lists**: Follow on-page links (`a[href]`) for series or chapters.
- **Pagination**: Detect "next" links or page numbers and iterate until exhausted.
- **API/JSON**: Some sites expose JSON endpoints; prefer official APIs when available.
- **Sitemaps**: Parse XML sitemaps for content discovery.
- Avoid blind numeric enumeration; prefer explicit links and manifests.

### 7) Parsing and extraction
- Prefer robust HTML parsing (e.g., `encoding/xml` for XML, `encoding/json` for JSON). Regex for narrow, well-understood patterns only.
- Normalize and de-duplicate discovered items (use maps/sets).
- Validate IDs/URLs (length, allowed characters) before enqueueing.

### 8) Downloads and storage
- Use `EnsureDir` for directories and sanitize file/dir names.
- Stream responses to disk with `io.Copy` (already in `DownloadFile`).
- Avoid loading large files fully into memory.

#### Safe file/dir name helper
```go
func safeName(s string) string {
    s = strings.TrimSpace(s)
    replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-")
    s = replacer.Replace(s)
    if len(s) == 0 { s = "untitled" }
    return s
}
```

### 9) Retry and backoff (transient vs terminal)
- Retry transient errors: timeouts, 5xx.
- Do not retry terminal errors: 401/403/404.
- Honor `Retry-After` when present.

```go
func isTransient(status int) bool { return status == 0 || status >= 500 }
```

### 10) Error handling patterns
- Return contextual errors (include the URL or resource identifier).
- Use `LogDebug` for noisy details, `LogInfo` for milestones, `LogWarn` for recoverable issues, `LogError` for failures.
- Stop on repeated policy violations (403/429).

### 11) Testing your adapter
- Add a dry-run mode that lists discovered items without downloading.
- Use `httptest.Server` to simulate site responses for unit/integration tests.
- Mock `Fetcher` to deterministically test edge cases and retries.

```go
type FakeFetcher struct{ R *http.Response; E error }
func (f *FakeFetcher) Get(url string, h map[string]string) (*http.Response, error) { return f.R, f.E }
```

### 12) Documentation template (recommended)
Create a short adapter guide in `adapters/<yoursite>/README.md`:

```
# <Your Site> Adapter

- Base URL: https://example.com
- Capabilities: discovery (series/chapters), downloads (if permitted), metadata
- Config hints: workers, timeouts
- Politeness: robots.txt, request rate, headers
- Known limitations and troubleshooting tips
```


## 🔧 Configuration

`Config` in `types.go` controls concurrency, timeouts, and retries:

```go
type Config struct {
    MaxWorkers        int
    MaxSeriesWorkers  int
    MaxChapterWorkers int
    MaxImageWorkers   int
    HTTPTimeout       time.Duration
    MaxRetries        int
    RetryDelay        time.Duration
}
```

Tune these conservatively per site. Many sites will require much lower concurrency and longer timeouts.

## ⚡ Performance Highlights

- **Bounded Concurrency**: `WorkerPool` limits parallel work per stage (series/chapters/images) to prevent overload.
- **Global Rate Limiting**: Token bucket example to cap overall request rate.
- **Streamed I/O**: `io.Copy` avoids buffering files in memory for large downloads.
- **Targeted Retries**: Retries are limited to transient errors with exponential backoff, reducing wasted load.
- **Backpressure-Friendly**: Acquire/Release patterns ensure upstream stages don’t outpace downstream capacity.
- **Minimal Dependencies**: Leverages standard library for speed and portability.
- **Config-Driven**: Tune concurrency and timeouts without code changes.

## 🧱 Modularity Highlights

- **Interface-Driven**: `SiteScraper` and `Fetcher` cleanly decouple core logic from site specifics.
- **Swappable Fetchers**: Replace HTTP client (e.g., for testing, proxies, caching) without touching adapters.
- **Composable Pools**: `WorkerPool` primitives are reusable across stages.
- **Clear Separation**: Discovery, parsing, and downloading are distinct concerns in adapter code.
- **Adapter Registration**: Adding a site is a local change (new file + switch case in `main.go`).
- **Testability**: `Fetcher` abstraction enables deterministic unit tests.

## 🐛 Troubleshooting

- Build issues: re-run `./build.sh` or `go build -o scraper *.go`
- Network issues: lower `-workers`, raise timeouts, and run with `-log=debug`
- Site blocks: stop immediately on 403/429; respect `Retry-After` and site policies

## ⚖️ Legal & Ethical Guidelines

- Always review and respect a site’s Terms of Service and `robots.txt`.
- Do not bypass authentication, CAPTCHAs, paywalls, or technical protection measures.
- Avoid scraping personal data; comply with privacy laws where applicable.
- Prefer metadata-only adapters when content replication is not permitted.
- Consult counsel for uncertain use cases. This repository is provided for educational and lawful use.

## 🤝 Contributing

- Keep adapters modular and self-contained.
- Favor clarity and maintainability over cleverness.
- Provide concise docs for each adapter you add.
- Ensure polite defaults and safe error handling.

---

Built with ❤️ for learning and responsible data access.
# Universal Manga/Manhwa Scraper

A high-performance, modular Go scraper supporting multiple manga/manhwa sites with dedicated adapters and comprehensive documentation.

## 🏗️ Project Structure

```
/
├── 📁 adapters/              # Organized adapter documentation
│   ├── 📁 comick/           # Comick.live adapter docs
│   │   ├── comick_adapter.go  # (source copy)
│   │   └── README.md         # Detailed Comick documentation
│   └── 📁 asura/            # AsuraComic.net adapter docs  
│       ├── asura_adapter.go   # (source copy)
│       └── README.md         # Detailed Asura documentation
├── 🔧 Core Files
│   ├── main.go              # CLI interface and configuration
│   ├── interfaces.go        # Site scraper interfaces
│   ├── types.go             # Data structures and config
│   ├── utils.go             # Utility functions and logging
│   ├── fetcher.go           # HTTP client abstraction
│   ├── comick_adapter.go    # Comick.live implementation
│   ├── asura_adapter.go     # AsuraComic.net implementation
│   ├── build.sh             # Build script
│   ├── go.mod               # Go module definition
│   └── README.md            # This file
└── 📦 Generated
    └── scraper              # Compiled binary
```

## 🚀 Quick Start

### Build
```bash
# Automated build (recommended)
chmod +x build.sh
./build.sh

# Manual build
go build -o scraper *.go
```

### Basic Usage
```bash
# Get help
./scraper -h

# Single site downloads
./scraper -site=comick -mode=slug -slug=solo-leveling
./scraper -site=asura -mode=slug -slug=reaper-of-the-drifting-moon-4e28152d

# Multi-site concurrent downloads (NEW!)
./scraper -site=all -mode=slug -slug=solo-leveling
./scraper -site=all -mode=full -workers=40

# Bulk downloads  
./scraper -site=comick -mode=full -workers=40
./scraper -site=asura -mode=full -workers=30
```

## 🚀 **NEW: Concurrent Multi-Site Scraping**

The scraper now supports **simultaneous downloads from all supported sites**! Use `-site=all` to scrape from Comick and AsuraComic concurrently.

### 🌟 **Multi-Site Benefits:**
- ⚡ **2x Speed**: Download from both sites simultaneously
- 🔄 **Parallel Processing**: Each site runs in its own goroutine
- 📊 **Progress Tracking**: Real-time status for each site
- 🛡️ **Error Isolation**: One site failing doesn't stop the other
- 📝 **Comprehensive Logging**: Per-site success/failure reporting

### 💡 **Multi-Site Examples:**
```bash
# Download the same series from both sites simultaneously
./scraper -site=all -mode=slug -slug=solo-leveling

# Full download from ALL sites (use with caution!)
./scraper -site=all -mode=full -workers=40

# Comick after-id + Asura full (after-id only applies to Comick)
./scraper -site=all -mode=after-id -start-id=1000
```

### 📋 **Multi-Site Behavior:**
- **`-mode=slug`**: Downloads the same slug from both sites
- **`-mode=full`**: Downloads entire databases from both sites
- **`-mode=after-id`**: Downloads after ID from Comick + full from Asura
- **Error handling**: Sites that fail are logged but don't stop others
- **Output**: Downloads are organized by series name (may have duplicates if both sites have the same series)

## 📊 Supported Sites

| Site | Adapter | Base URL | Type | Features | Documentation |
|------|---------|----------|------|----------|---------------|
| **Comick** | `comick` | comick.live | API-based | Hash discovery, Pagination, High concurrency | [📖 Details](adapters/comick/README.md) |
| **AsuraComic** | `asura` | asuracomic.net | HTML parsing | Sequential chapters, CDN optimization | [📖 Details](adapters/asura/README.md) |
| **🌐 Multi-Site** | `all` | All supported | Concurrent | **SIMULTANEOUS SCRAPING** from all sites | 🚀 **NEW!** |

## 🔧 Configuration Options

| Option | Description | Values | Default |
|--------|-------------|--------|---------|
| `-site` | Site to scrape | `comick`, `asura`, **`all`** | **Required** |
| `-mode` | Download mode | `full`, `slug`, `after-id` | **Required** |
| `-slug` | Series identifier | Site-specific slug | For slug mode |
| `-start-id` | Starting ID | Number ≥ 1 | Comick only |
| `-workers` | Concurrent workers | 1-50 | 20 |
| `-log` | Log level | `debug`, `info`, `warn`, `error` | `info` |

## 📁 Output Structure

All downloads are organized in a consistent structure:

```
downloads/
├── solo-leveling/              # Series folder (slug-based)
│   ├── cover.webp             # Cover image
│   ├── chapter_1/             # Chapter folders (1-based)
│   │   ├── 000.webp          # Page images (0-based)
│   │   ├── 001.webp
│   │   └── ...
│   ├── chapter_2/
│   └── ...
└── another-series/
    └── ...
```

## 🌟 Key Features

### 🏛️ **Modular Architecture**
- **Clean interfaces**: Common `SiteScraper` interface for all adapters
- **Site-specific optimizations**: Each adapter handles site quirks perfectly
- **Easy extensibility**: Add new sites without affecting existing ones
- **Comprehensive documentation**: Detailed docs for each adapter

### ⚡ **High Performance**
- **Parallel processing**: Concurrent downloads at series, chapter, and image levels
- **Smart worker pools**: Configurable concurrency for optimal performance
- **Rate limiting**: Server-friendly request patterns
- **Automatic retry**: Exponential backoff for failed requests

### 🛡️ **Robust Error Handling**
- **Graceful failures**: Continues processing when individual items fail
- **Smart termination**: Stops after consecutive 404s
- **Comprehensive logging**: Debug, info, warn, and error levels
- **Recovery strategies**: Built-in retry logic with exponential backoff

### 🔍 **Site-Specific Optimizations**

#### Comick.live (`-site=comick`)
- ✅ **API-based**: Fast and reliable data access
- ✅ **Advanced hash discovery**: Multiple fallback strategies
- ✅ **Pagination support**: Handles large chapter lists
- ✅ **Bulk operations**: `after-id` mode for selective downloads

#### AsuraComic.net (`-site=asura`)  
- ✅ **HTML parsing**: Robust pattern matching
- ✅ **CDN optimization**: Direct access to image CDN
- ✅ **Sequential discovery**: Smart chapter enumeration
- ✅ **Cover extraction**: Automatic cover image downloading

## 📚 Detailed Documentation

Each adapter has comprehensive documentation covering:

### 🔷 [Comick.live Adapter](adapters/comick/README.md)
- API endpoints and response formats
- Hash discovery algorithms  
- Performance benchmarks
- Troubleshooting guide
- Technical implementation details

### 🔷 [AsuraComic.net Adapter](adapters/asura/README.md)
- HTML parsing strategies
- URL pattern matching
- Site-specific optimizations
- Error handling approaches
- Development notes

## ⚡ Performance Guide

### Recommended Configurations

| Use Case | Command | Workers | Notes |
|----------|---------|---------|-------|
| **Single Series** | `./scraper -site=comick -mode=slug -slug=series` | 25-30 | Fast, reliable |
| **🌐 Multi-Site Series** | `./scraper -site=all -mode=slug -slug=series` | **30-35** | **2x speed!** |
| **Bulk Download** | `./scraper -site=comick -mode=after-id -start-id=1000` | 35-40 | High throughput |
| **🌐 Multi-Site Full** | `./scraper -site=all -mode=full` | **35-40** | **Maximum throughput** |
| **Full Site** | `./scraper -site=asura -mode=full` | 25-30 | Respectful load |
| **Conservative** | `./scraper -site=* -mode=* -workers=15` | 15 | Slow connections |

### Performance Characteristics

| Site | Series Discovery | Chapter Processing | Image Downloads | Avg Speed |
|------|-----------------|-------------------|-----------------|-----------|
| **Comick** | 100 pages parallel | API-based, fast | Hash discovery | ⭐⭐⭐⭐⭐ |
| **Asura** | 20 pages parallel | Sequential, reliable | Direct CDN | ⭐⭐⭐⭐ |
| **🌐 Multi-Site** | **CONCURRENT** | **SIMULTANEOUS** | **PARALLEL** | **⭐⭐⭐⭐⭐⭐** |

## 🔨 Adding New Sites

The modular architecture makes adding new sites straightforward:

### 1. Create Adapter Structure
```bash
mkdir -p adapters/newsite
```

### 2. Implement the Interface
```go
// newsite_adapter.go
type NewSiteAdapter struct {
    // ... implementation
}

func (n *NewSiteAdapter) DownloadAll() error { /* ... */ }
func (n *NewSiteAdapter) DownloadBySlug(slug string) error { /* ... */ }
func (n *NewSiteAdapter) GetSiteName() string { return "newsite" }
```

### 3. Register in Main
```go
// main.go
case "newsite":
    scraper = NewNewSiteAdapter(config)
```

### 4. Create Documentation
```bash
# adapters/newsite/README.md
# Detailed adapter documentation
```

## 🐛 Troubleshooting

### Common Issues

**Build Problems:**
```bash
# Clean build
rm -f scraper
./build.sh
```

**Network Issues:**
```bash
# Reduce workers and enable debug logging
./scraper -site=comick -mode=slug -slug=test -workers=15 -log=debug
```

**Site-Specific Issues:**
- **Comick**: Check [Comick troubleshooting](adapters/comick/README.md#troubleshooting)
- **Asura**: Check [Asura troubleshooting](adapters/asura/README.md#troubleshooting)

### Finding Series Slugs

#### Comick.live
- URL: `https://comick.live/comic/solo-leveling`
- Slug: `solo-leveling`

#### AsuraComic.net  
- URL: `https://asuracomic.net/series/reaper-of-the-drifting-moon-4e28152d`
- Slug: `reaper-of-the-drifting-moon-4e28152d` (includes HID)

## 📈 Future Roadmap

### Planned Features
- [ ] **Database integration** for download tracking
- [ ] **Resume capability** for interrupted downloads
- [ ] **Web UI** for management and monitoring
- [ ] **Docker containerization** for easy deployment
- [ ] **Advanced filtering** by genre, status, date ranges
- [ ] **Quality selection** for different image sizes

### Potential New Sites
- [ ] **MangaDex** adapter
- [ ] **Webtoons** adapter  
- [ ] **MangaKakalot** adapter
- [ ] **Community-requested sites**

## ⚖️ Legal & Ethical Usage

### Guidelines
- ✅ **Educational purposes** - Learn about web scraping and Go programming
- ✅ **Personal use** - Download for your own reading
- ✅ **Respectful scraping** - Built-in rate limiting and server-friendly patterns
- ✅ **Terms compliance** - Follow each site's terms of service

### Built-in Protections
- **Rate limiting**: Prevents server overload
- **Retry logic**: Handles temporary issues gracefully  
- **Error recovery**: Continues processing despite individual failures
- **Logging**: Comprehensive debugging without exposing sensitive data

## 🤝 Contributing

### Code Quality Standards
- **Clean architecture**: Modular, testable, maintainable
- **Comprehensive docs**: Each adapter thoroughly documented
- **Error handling**: Graceful failure recovery
- **Performance**: Optimized for speed and efficiency

### Development Setup
```bash
# Clone and setup
git clone <repository>
cd scraper

# Build and test
./build.sh
./scraper -h

# Test adapters
./scraper -site=comick -mode=slug -slug=test-series -log=debug
./scraper -site=asura -mode=slug -slug=test-series -log=debug
```

---

## 🏆 Project Highlights

### 🎯 **Production Ready**
- Comprehensive error handling and logging
- Server-friendly rate limiting and retry logic
- Modular architecture for easy maintenance
- Extensive documentation for all components

### 🚀 **High Performance**  
- **🌐 Multi-site concurrent scraping**: Download from all sites simultaneously
- Parallel processing at all levels
- Site-specific optimizations
- Configurable concurrency control
- Smart resource management

### 📚 **Well Documented**
- Main project documentation
- Detailed adapter-specific guides
- Troubleshooting and performance guides
- Clear examples and usage patterns

### 🔧 **Extensible Design**
- Clean interface-based architecture
- Easy to add new sites
- Modular components
- Future-proof design patterns

---

**Built with ❤️ for the manga/manhwa community**

*Combining enterprise architecture with bleeding-edge performance*

**🌐 Concurrent Multi-Site** • **⚡ High-Performance** • **📚 Well-Documented** • **🔧 Extensible**
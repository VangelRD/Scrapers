// main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

func printUsage() {
	programName := "./comick-scraper"
	fmt.Printf(`Comick.live Manhwa Scraper - High-Performance Edition

USAGE:
    %s [OPTIONS]

MODES:
    -mode=full                Download ALL manhwas from comick.live
    -mode=slug                Download specific manhwa by slug
    -mode=after-id            Download manhwas with ID >= specified number

OPTIONS:
    -slug=STRING              Manhwa slug (required for -mode=slug)
    -start-id=NUMBER          Starting manhwa ID (required for -mode=after-id)
    -workers=NUMBER           Concurrent download workers (default: 20, max: 50)
    -log=STRING               Log level (info, warn, error, debug)

EXAMPLES:
    %s -mode=slug -slug=solo-leveling
    %s -mode=after-id -start-id=500 -workers=30
    %s -mode=full -workers=40

PERFORMANCE:
    - Parallel page discovery: 100 pages simultaneously
    - Parallel manhwa processing: 10 concurrent
    - Parallel chapter processing: 5 per manhwa
    - Parallel image downloads: configurable (20-50)
    - Target: <1 hour for full database download

OUTPUT STRUCTURE:
    downloads/
    ├── manhwa-slug/
    │   ├── cover.webp
    │   ├── chapter_1/
    │   │   ├── 000.webp
    │   │   └── ...
    │   └── chapter_2/
    └── ...

`, programName, programName, programName, programName)
}

func main() {
	var (
		mode     = flag.String("mode", "", "Required: 'full', 'slug', or 'after-id'")
		slug     = flag.String("slug", "", "Manhwa slug (for -mode=slug)")
		startID  = flag.Int("start-id", 0, "Starting manhwa ID (for -mode=after-id)")
		workers  = flag.Int("workers", 20, "Concurrent download workers (1-50, default 20)")
		logLevel = flag.String("log", "info", "Log level: info, warn, error, debug")
		help     = flag.Bool("h", false, "Show help")
		help2    = flag.Bool("help", false, "Show help")
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

	// Setup structured logging
	SetupLogger(*logLevel)

	// Validate and limit workers
	if *workers > 50 {
		log.Printf("Warning: Worker count limited to 50 (requested: %d)", *workers)
		*workers = 50
	} else if *workers < 1 {
		log.Printf("Warning: Worker count must be at least 1 (requested: %d)", *workers)
		*workers = 1
	}

	// Create scraper with optimized config
	config := Config{
		MaxWorkers:        *workers,
		MaxManhwaWorkers:  10,
		MaxChapterWorkers: 5,
		MaxImageWorkers:   *workers,
		HTTPTimeout:       15 * time.Second,
		PageBatchSize:     100, // High-performance page discovery
		MaxRetries:        3,
		RetryDelay:        time.Second,
	}

	scraper := NewComickScraper(config)

	switch *mode {
	case "full":
		log.Println("Starting full download of all manhwas...")
		if err := scraper.DownloadAllManhwas(); err != nil {
			log.Fatalf("Fatal: %v", err)
		}
	case "slug":
		if *slug == "" {
			log.Fatal("Please provide a slug with -slug=your-manhwa-slug")
		}
		if err := scraper.DownloadManhwaBySlug(*slug); err != nil {
			log.Fatalf("Fatal: %v", err)
		}
	case "after-id":
		if *startID <= 0 {
			log.Fatal("Please provide a valid start ID with -start-id=123")
		}
		if err := scraper.DownloadManhwasAfterID(*startID); err != nil {
			log.Fatalf("Fatal: %v", err)
		}
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}

	log.Println("Scraping completed!")
}

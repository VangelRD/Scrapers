// main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

func printUsage() {
	fmt.Printf(`Universal Manga/Manhwa Scraper - Multi-Site Edition

SUPPORTED SITES:
    - comick.live (comick)
    - asuracomic.net (asura)
    - ALL SITES SIMULTANEOUSLY (all)

USAGE:
    ./scraper -site=SITE [OPTIONS]

MODES:
    -mode=full                Download ALL series from the site(s)
    -mode=slug                Download specific series by slug/id
    -mode=after-id            Download series with ID >= specified (comick only)

OPTIONS:
    -site=STRING              Site to scrape (comick, asura, all) [REQUIRED]
    -slug=STRING              Series slug/id (required for -mode=slug)
    -start-id=NUMBER          Starting ID (for -mode=after-id, comick only)
    -workers=NUMBER           Concurrent download workers (default: 20)
    -log=STRING               Log level (debug, info, warn, error)

EXAMPLES:
    # Single site scraping
    ./scraper -site=comick -mode=slug -slug=solo-leveling
    ./scraper -site=asura -mode=slug -slug=reaper-of-the-drifting-moon-4e28152d
    ./scraper -site=comick -mode=full -workers=40
    ./scraper -site=asura -mode=full -workers=30
    
    # Multi-site scraping (CONCURRENT!)
    ./scraper -site=all -mode=full -workers=40
    ./scraper -site=all -mode=slug -slug=solo-leveling

OUTPUT:
    downloads/
    ├── [series-slug]/
    │   ├── cover.webp
    │   ├── chapter_1/
    │   │   ├── 000.webp
    │   │   └── ...
    │   └── ...
    └── ...
`)
}

// runMultiSiteScraping runs multiple site adapters concurrently
func runMultiSiteScraping(config Config, mode, slug string, startID int) error {
	log.Println("🚀 Starting concurrent multi-site scraping...")

	// Create all adapters
	adapters := map[string]SiteScraper{
		"comick": NewComickAdapter(config),
		"asura":  NewAsuraAdapter(config),
	}

	var wg sync.WaitGroup
	errorChan := make(chan error, len(adapters))

	// Launch each adapter in a separate goroutine
	for siteName, adapter := range adapters {
		wg.Add(1)
		go func(site string, scraper SiteScraper) {
			defer wg.Done()

			log.Printf("📥 [%s] Starting scraper...", site)
			var err error

			switch mode {
			case "full":
				log.Printf("📚 [%s] Starting full download...", site)
				err = scraper.DownloadAll()
			case "slug":
				log.Printf("📖 [%s] Downloading series: %s", site, slug)
				err = scraper.DownloadBySlug(slug)
			case "after-id":
				if site == "comick" {
					log.Printf("🔢 [%s] Starting download after ID %d...", site, startID)
					if comickScraper, ok := scraper.(*ComickAdapter); ok {
						err = comickScraper.DownloadAfterID(startID)
					}
				} else {
					log.Printf("⚠️  [%s] Skipping after-id mode (not supported)", site)
					return // Skip this adapter
				}
			}

			if err != nil {
				log.Printf("❌ [%s] Failed: %v", site, err)
				errorChan <- fmt.Errorf("[%s] %v", site, err)
			} else {
				log.Printf("✅ [%s] Completed successfully!", site)
			}
		}(siteName, adapter)
	}

	// Wait for all adapters to complete
	wg.Wait()
	close(errorChan)

	// Collect any errors
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		log.Printf("⚠️  Multi-site scraping completed with %d errors:", len(errors))
		for _, err := range errors {
			log.Printf("   - %v", err)
		}
		return fmt.Errorf("multi-site scraping had %d errors", len(errors))
	}

	log.Println("🎉 Multi-site scraping completed successfully for all sites!")
	return nil
}

func main() {
	var (
		site     = flag.String("site", "", "Site to scrape (comick, asura, all)")
		mode     = flag.String("mode", "", "Mode: full, slug, or after-id")
		slug     = flag.String("slug", "", "Series slug/id")
		startID  = flag.Int("start-id", 0, "Starting ID (comick only)")
		workers  = flag.Int("workers", 20, "Concurrent workers (1-50)")
		logLevel = flag.String("log", "info", "Log level")
		help     = flag.Bool("h", false, "Show help")
	)

	flag.Usage = printUsage
	flag.Parse()

	if *help || *site == "" || *mode == "" {
		printUsage()
		if *site == "" && !*help {
			fmt.Println("\nError: -site parameter is required")
		}
		if *mode == "" && !*help {
			fmt.Println("Error: -mode parameter is required")
		}
		os.Exit(1)
	}

	SetupLogger(*logLevel)

	// Validate workers
	if *workers > 50 {
		*workers = 50
		log.Printf("Worker count limited to 50")
	} else if *workers < 1 {
		*workers = 1
	}

	config := Config{
		MaxWorkers:        *workers,
		MaxSeriesWorkers:  10,
		MaxChapterWorkers: 5,
		MaxImageWorkers:   *workers,
		HTTPTimeout:       15 * time.Second,
		MaxRetries:        3,
		RetryDelay:        time.Second,
	}

	// Handle multi-site scraping
	if *site == "all" {
		log.Println("🌐 Multi-site mode activated!")

		// Validate mode for multi-site
		if *mode == "slug" && *slug == "" {
			log.Fatal("Please provide a slug with -slug=series-slug for multi-site slug mode")
		}
		if *mode == "after-id" && *startID <= 0 {
			log.Fatal("Please provide a valid start ID with -start-id=123 for after-id mode")
		}

		err := runMultiSiteScraping(config, *mode, *slug, *startID)
		if err != nil {
			log.Fatalf("Multi-site scraping failed: %v", err)
		}
		log.Println("Multi-site scraping completed!")
		return
	}

	// Single site scraping (existing logic)
	var scraper SiteScraper

	switch *site {
	case "comick":
		scraper = NewComickAdapter(config)
		log.Println("Initialized Comick.live scraper")
	case "asura":
		scraper = NewAsuraAdapter(config)
		log.Println("Initialized AsuraComic.net scraper")
	default:
		log.Fatalf("Unknown site: %s (supported: comick, asura, all)", *site)
	}

	// Execute based on mode
	var err error
	switch *mode {
	case "full":
		log.Printf("Starting full download from %s...", *site)
		err = scraper.DownloadAll()
	case "slug":
		if *slug == "" {
			log.Fatal("Please provide a slug with -slug=series-slug")
		}
		log.Printf("Downloading %s from %s...", *slug, *site)
		err = scraper.DownloadBySlug(*slug)
	case "after-id":
		if *site != "comick" {
			log.Fatal("-mode=after-id is only supported for comick site")
		}
		if *startID <= 0 {
			log.Fatal("Please provide a valid start ID with -start-id=123")
		}
		if comickScraper, ok := scraper.(*ComickAdapter); ok {
			err = comickScraper.DownloadAfterID(*startID)
		}
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}

	if err != nil {
		log.Fatalf("Scraping failed: %v", err)
	}

	log.Println("Scraping completed successfully!")
}

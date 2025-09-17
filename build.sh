#!/bin/bash

# Build the comick scraper
echo "Building comick scraper..."
go build -o comick-scraper main.go

# Make it executable
chmod +x comick-scraper

echo "Build complete! The scraper is ready to use."
echo ""
echo "Usage examples:"
echo "  Full download:    ./comick-scraper -mode=full"
echo "  Specific manhwa:  ./comick-scraper -mode=slug -slug=your-manhwa-slug"
echo "  After ID:         ./comick-scraper -mode=after-id -start-id=1000"
echo ""
echo "Use -workers=N to control concurrent downloads (default: 10)"

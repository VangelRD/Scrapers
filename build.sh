#!/bin/bash

echo "Building high-performance comick scraper..."

# Build the scraper
go build -o comick-scraper *.go

# Make it executable
chmod +x comick-scraper

echo "Build complete!"
echo ""
echo "Usage examples:"
echo "  Full download:    ./comick-scraper -mode=full -workers=40"
echo "  Specific manhwa:  ./comick-scraper -mode=slug -slug=solo-leveling"
echo "  After ID:         ./comick-scraper -mode=after-id -start-id=1000 -workers=30"
echo ""
echo "Performance features:"
echo "  - Parallel page discovery (100 pages simultaneously)"
echo "  - Parallel manhwa processing (10 concurrent)"
echo "  - Parallel chapter processing (5 per manhwa)"
echo "  - Configurable image download workers (20-50)"
echo "  - Advanced hash discovery algorithms"
echo "  - Automatic retry logic with exponential backoff"
echo "  - Smart pagination support"
echo ""
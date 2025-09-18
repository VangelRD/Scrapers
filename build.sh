#!/bin/bash

echo "Building Universal Manga/Manhwa Scraper..."
echo ""

# Check for Go installation
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.20+ first."
    exit 1
fi

# Clean old build
rm -f scraper comick-scraper

# Build the application
echo "Compiling..."
go build -o scraper *.go

if [ $? -eq 0 ]; then
    chmod +x scraper
    echo ""
    echo "✅ Build successful!"
    echo ""
    echo "Supported sites:"
    echo "  • comick.live  (-site=comick)"
    echo "  • asuracomic.net (-site=asura)"
    echo "  • 🌐 ALL SITES SIMULTANEOUSLY (-site=all)"
    echo ""
    echo "Quick examples:"
    echo "  # Single site"
    echo "  ./scraper -site=comick -mode=slug -slug=solo-leveling"
    echo "  ./scraper -site=asura -mode=slug -slug=reaper-of-the-drifting-moon-4e28152d"
    echo "  # Multi-site concurrent scraping (NEW!)"
    echo "  ./scraper -site=all -mode=slug -slug=solo-leveling"
    echo "  ./scraper -site=all -mode=full -workers=40"
    echo ""
    echo "For help: ./scraper -h"
else
    echo ""
    echo "❌ Build failed! Check error messages above."
    exit 1
fi
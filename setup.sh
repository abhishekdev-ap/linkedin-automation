#!/bin/bash
# LinkedIn Automation Tool - Setup and Test Script
# This script helps set up and verify the project

set -e

echo "=========================================="
echo "LinkedIn Automation Tool - Setup Script"
echo "=========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check Go installation
echo "1. Checking Go installation..."
if command -v go &> /dev/null; then
    GO_VERSION=$(go version)
    echo -e "${GREEN}✓ Go is installed: $GO_VERSION${NC}"
else
    echo -e "${RED}✗ Go is not installed${NC}"
    echo ""
    echo "Please install Go using one of these methods:"
    echo ""
    echo "  Option 1: Using Homebrew (Recommended)"
    echo "    brew install go"
    echo ""
    echo "  Option 2: Download from official site"
    echo "    https://go.dev/dl/"
    echo ""
    exit 1
fi

echo ""

# Check environment file
echo "2. Checking environment configuration..."
if [ -f .env ]; then
    echo -e "${GREEN}✓ .env file exists${NC}"
else
    if [ -f .env.example ]; then
        echo -e "${YELLOW}! Creating .env from template...${NC}"
        cp .env.example .env
        echo -e "${YELLOW}! Please edit .env with your LinkedIn credentials${NC}"
    else
        echo -e "${RED}✗ No .env.example found${NC}"
    fi
fi

echo ""

# Download dependencies
echo "3. Downloading Go dependencies..."
go mod download
echo -e "${GREEN}✓ Dependencies downloaded${NC}"

echo ""

# Build the application
echo "4. Building the application..."
go build -o linkedin-bot ./cmd/linkedin-bot
echo -e "${GREEN}✓ Build successful: ./linkedin-bot${NC}"

echo ""

# Run basic tests
echo "5. Running basic verification..."
./linkedin-bot --help > /dev/null 2>&1
echo -e "${GREEN}✓ CLI is working${NC}"

echo ""
echo "=========================================="
echo -e "${GREEN}Setup Complete!${NC}"
echo "=========================================="
echo ""
echo "Next steps:"
echo ""
echo "  1. Edit your credentials:"
echo "     nano .env"
echo ""
echo "  2. Run the login command:"
echo "     ./linkedin-bot login"
echo ""
echo "  3. Search for profiles:"
echo "     ./linkedin-bot search --title \"Software Engineer\" --pages 2"
echo ""
echo "  4. View statistics:"
echo "     ./linkedin-bot stats"
echo ""
echo "For help:"
echo "  ./linkedin-bot --help"
echo ""

#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🔍 Starting Development Environment Check...${NC}"

# 1. Check Go Version
echo -e "\n${YELLOW}[1/6] Checking Go version...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed.${NC}"
    exit 1
fi
GO_VERSION=$(go version | awk '{print $3}')
echo -e "${GREEN}✅ Found Go version: ${GO_VERSION}${NC}"

# 2. Check Configuration Files
echo -e "\n${YELLOW}[2/6] Checking configuration files...${NC}"
MISSING_CONFIG=0

if [ ! -f .env ]; then
    echo -e "${RED}❌ .env file is missing.${NC}"
    echo "   👉 Run: cp example.env .env"
    MISSING_CONFIG=1
else
    echo -e "${GREEN}✅ .env exists.${NC}"
fi

if [ ! -f config.yml ]; then
    echo -e "${RED}❌ config.yml file is missing.${NC}"
    MISSING_CONFIG=1
else
    echo -e "${GREEN}✅ config.yml exists.${NC}"
fi

if [ $MISSING_CONFIG -eq 1 ]; then
    echo -e "${RED}⚠️  Please create missing configuration files before running the bot.${NC}"
    # We don't exit here to allow running other checks, but prompt user
fi

# 3. Go Mod Tidy
echo -e "\n${YELLOW}[3/6] Running go mod tidy...${NC}"
go mod tidy
echo -e "${GREEN}✅ Dependencies are tidy.${NC}"

# 4. Go Format
echo -e "\n${YELLOW}[4/6] Checking code formatting...${NC}"
# list files that need formatting
FMT_FILES=$(gofmt -l .)
if [ -n "$FMT_FILES" ]; then
    echo -e "${RED}❌ The following files need formatting:${NC}"
    echo "$FMT_FILES"
    echo "   👉 Run: go fmt ./..."
    exit 1
else
    echo -e "${GREEN}✅ Code formatting is correct.${NC}"
fi

# 5. Go Vet
echo -e "\n${YELLOW}[5/6] Running go vet...${NC}"
if go vet ./...; then
    echo -e "${GREEN}✅ Static analysis passed.${NC}"
else
    echo -e "${RED}❌ Static analysis failed.${NC}"
    exit 1
fi

# 6. Run Tests
echo -e "\n${YELLOW}[6/6] Running tests...${NC}"
if command -v gotestsum &> /dev/null; then
    echo "Using gotestsum for formatted output..."
    gotestsum --format pkgname
else
    echo "gotestsum not found, using go test..."
    go test ./...
fi

echo -e "\n${GREEN}✨ All checks completed! You are ready to develop.${NC}"

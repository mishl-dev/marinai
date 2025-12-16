#!/bin/bash
set -e

# Toolsmith Dev Check 🔨
# Validates the development environment.

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🔨 Toolsmith Dev Check starting...${NC}"

# 1. Go Version
echo -n "Checking Go version... "
if ! command -v go &> /dev/null; then
    echo -e "${RED}FAILED${NC}"
    echo "Go is not installed."
    exit 1
fi
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo -e "${GREEN}OK (${GO_VERSION})${NC}"

# 2. Config Files
echo -n "Checking configuration... "
if [ ! -f .env ]; then
    echo -e "${RED}FAILED${NC}"
    echo ".env file missing. Copy example.env to .env and fill in values."
    exit 1
fi
if [ ! -f config.yml ]; then
    echo -e "${RED}FAILED${NC}"
    echo "config.yml file missing."
    exit 1
fi

# Check for missing keys in .env
MISSING_KEYS=0
while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip comments and empty lines
    if [[ "$line" =~ ^# ]] || [[ -z "$line" ]]; then
        continue
    fi
    KEY=$(echo "$line" | cut -d '=' -f 1)
    if ! grep -q "^$KEY=" .env; then
        echo -e "\n${YELLOW}Warning: Key $KEY missing in .env${NC}"
        MISSING_KEYS=1
    fi
done < example.env

if [ $MISSING_KEYS -eq 0 ]; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${YELLOW}Config check passed with warnings.${NC}"
fi

# 3. Code Formatting
echo -n "Checking code formatting... "
UNFORMATTED=$(gofmt -l .)
if [ -n "$UNFORMATTED" ]; then
    echo -e "${RED}FAILED${NC}"
    echo "The following files are not formatted:"
    echo "$UNFORMATTED"
    echo "Run 'go fmt ./...' to fix."
    exit 1
fi
echo -e "${GREEN}OK${NC}"

# 4. Linting (go vet)
echo -n "Running go vet... "
if go vet ./...; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${RED}FAILED${NC}"
    exit 1
fi

# 5. Tests
RUN_TESTS=true
TEST_ARGS="-short"

for arg in "$@"; do
    case $arg in
        --all)
        TEST_ARGS=""
        ;;
        --skip-tests)
        RUN_TESTS=false
        ;;
    esac
done

if [ "$RUN_TESTS" = true ]; then
    echo -n "Running tests (args: $TEST_ARGS)... "
    # Capture output to file to keep screen clean, show on failure
    if go test $TEST_ARGS ./... > test_output.log 2>&1; then
        echo -e "${GREEN}OK${NC}"
        rm test_output.log
    else
        echo -e "${RED}FAILED${NC}"
        cat test_output.log
        rm test_output.log
        exit 1
    fi
else
    echo -e "${YELLOW}Skipping tests${NC}"
fi

echo -e "${GREEN}✨ All checks passed! Ready to code.${NC}"

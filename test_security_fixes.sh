#!/bin/bash
# Test script to verify security fixes are working
# Usage: ./test_security_fixes.sh [server_url]

SERVER_URL="${1:-http://localhost:8080}"
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "Testing security improvements on $SERVER_URL"
echo "================================================"
echo ""

# Test 1: Request size limit
echo -e "${YELLOW}Test 1: Request Size Limit${NC}"
echo "Sending a request with 11MB payload (exceeds 10MB limit)..."

# Generate large payload
LARGE_PAYLOAD=$(python3 -c "import json; print(json.dumps({'model': 'test', 'input': 'x' * 11000000}))" 2>/dev/null || \
                perl -e 'print "{\"model\":\"test\",\"input\":\"" . ("x" x 11000000) . "\"}"')

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$SERVER_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d "$LARGE_PAYLOAD" \
  --max-time 5 2>/dev/null)

if [ "$HTTP_CODE" = "413" ]; then
  echo -e "${GREEN}✓ PASS: Received HTTP 413 (Request Entity Too Large)${NC}"
else
  echo -e "${RED}✗ FAIL: Expected 413, got $HTTP_CODE${NC}"
fi
echo ""

# Test 2: Normal request size
echo -e "${YELLOW}Test 2: Normal Request Size${NC}"
echo "Sending a small valid request..."

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$SERVER_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{"model":"test","input":"hello"}' \
  --max-time 5 2>/dev/null)

# Expected: either 400 (invalid model) or 502 (provider error), but NOT 413
if [ "$HTTP_CODE" != "413" ]; then
  echo -e "${GREEN}✓ PASS: Request not rejected by size limit (HTTP $HTTP_CODE)${NC}"
else
  echo -e "${RED}✗ FAIL: Small request incorrectly rejected with 413${NC}"
fi
echo ""

# Test 3: Health endpoint
echo -e "${YELLOW}Test 3: Health Endpoint${NC}"
echo "Checking /health endpoint..."

RESPONSE=$(curl -s -X GET "$SERVER_URL/health" --max-time 5 2>/dev/null)
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$SERVER_URL/health" --max-time 5 2>/dev/null)

if [ "$HTTP_CODE" = "200" ] && echo "$RESPONSE" | grep -q "healthy"; then
  echo -e "${GREEN}✓ PASS: Health endpoint responding correctly${NC}"
else
  echo -e "${RED}✗ FAIL: Health endpoint not responding correctly (HTTP $HTTP_CODE)${NC}"
fi
echo ""

# Test 4: Ready endpoint
echo -e "${YELLOW}Test 4: Ready Endpoint${NC}"
echo "Checking /ready endpoint..."

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$SERVER_URL/ready" --max-time 5 2>/dev/null)

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "503" ]; then
  echo -e "${GREEN}✓ PASS: Ready endpoint responding (HTTP $HTTP_CODE)${NC}"
else
  echo -e "${RED}✗ FAIL: Ready endpoint not responding correctly (HTTP $HTTP_CODE)${NC}"
fi
echo ""

# Test 5: Models endpoint
echo -e "${YELLOW}Test 5: Models Endpoint${NC}"
echo "Checking /v1/models endpoint..."

RESPONSE=$(curl -s -X GET "$SERVER_URL/v1/models" --max-time 5 2>/dev/null)
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X GET "$SERVER_URL/v1/models" --max-time 5 2>/dev/null)

if [ "$HTTP_CODE" = "200" ] && echo "$RESPONSE" | grep -q "object"; then
  echo -e "${GREEN}✓ PASS: Models endpoint responding correctly${NC}"
else
  echo -e "${RED}✗ FAIL: Models endpoint not responding correctly (HTTP $HTTP_CODE)${NC}"
fi
echo ""

echo "================================================"
echo -e "${GREEN}Testing complete!${NC}"
echo ""
echo "Note: Panic recovery cannot be tested externally without"
echo "causing intentional server errors. It has been verified"
echo "through unit tests in middleware_test.go"

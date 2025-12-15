#!/bin/bash
# Test script for API Gateway header enrichment verification
# Uses mendhak/http-https-echo service to inspect headers passed through API Gateway

set -e

PORT=${1:-8080}
BASE_URL="http://localhost:$PORT"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "========================================"
echo "API Gateway Header Enrichment Test Suite"
echo "========================================"
echo "Target: $BASE_URL"
echo ""

# Test 1: Public endpoint (no auth headers expected)
echo -e "${YELLOW}Test 1: Public endpoint (EchoPublic)${NC}"
echo "  Endpoint: /api/EchoPublic/test"
echo "  Expected: 200 OK, no user-id/session-id headers"
response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/EchoPublic/test")
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

if [ "$http_code" = "200" ]; then
    echo -e "  Status: ${GREEN}$http_code OK${NC}"

    # Check for auth headers (should be absent)
    user_id=$(echo "$body" | jq -r '.headers["user-id"] // empty')
    session_id=$(echo "$body" | jq -r '.headers["session-id"] // empty')

    if [ -z "$user_id" ] && [ -z "$session_id" ]; then
        echo -e "  Auth headers: ${GREEN}Not present (expected)${NC}"
    else
        echo -e "  Auth headers: ${RED}Present unexpectedly (user-id=$user_id, session-id=$session_id)${NC}"
    fi

    # Show path rewrite worked
    path=$(echo "$body" | jq -r '.path')
    echo "  Path received by backend: $path"
else
    echo -e "  Status: ${RED}$http_code (expected 200)${NC}"
fi
echo ""

# Test 2: Protected endpoint without auth (should fail)
echo -e "${YELLOW}Test 2: Protected endpoint WITHOUT auth (EchoProtected)${NC}"
echo "  Endpoint: /api/EchoProtected/headers"
echo "  Expected: 401 Unauthorized"
http_code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/EchoProtected/headers")

if [ "$http_code" = "401" ]; then
    echo -e "  Status: ${GREEN}$http_code Unauthorized (expected)${NC}"
else
    echo -e "  Status: ${RED}$http_code (expected 401)${NC}"
fi
echo ""

# Test 3: Optional endpoint without auth
echo -e "${YELLOW}Test 3: Optional auth endpoint WITHOUT auth (EchoOptional)${NC}"
echo "  Endpoint: /api/EchoOptional/headers"
echo "  Expected: 200 OK, no user-id/session-id headers"
response=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/EchoOptional/headers")
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

if [ "$http_code" = "200" ]; then
    echo -e "  Status: ${GREEN}$http_code OK${NC}"

    user_id=$(echo "$body" | jq -r '.headers["user-id"] // empty')
    session_id=$(echo "$body" | jq -r '.headers["session-id"] // empty')

    if [ -z "$user_id" ] && [ -z "$session_id" ]; then
        echo -e "  Auth headers: ${GREEN}Not present (expected without auth)${NC}"
    else
        echo -e "  Auth headers: ${YELLOW}Present (user-id=$user_id, session-id=$session_id)${NC}"
    fi
else
    echo -e "  Status: ${RED}$http_code (expected 200)${NC}"
fi
echo ""

# Test 4: Show all headers from public endpoint
echo -e "${YELLOW}Test 4: All headers on public endpoint${NC}"
echo "  Headers added by API Gateway:"
curl -s "$BASE_URL/api/EchoPublic/headers" | jq -r '.headers | to_entries[] | select(.key | test("x-envoy|x-forwarded|traceparent|x-request-id")) | "    \(.key): \(.value)"'
echo ""

# Test 5: Test with session cookie (if available)
echo -e "${YELLOW}Test 5: Protected endpoint WITH auth cookie${NC}"
if [ -n "$SESSION_COOKIE" ]; then
    echo "  Using session cookie from \$SESSION_COOKIE env var"
    response=$(curl -s -w "\n%{http_code}" -H "Cookie: session=$SESSION_COOKIE" "$BASE_URL/api/EchoProtected/headers")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [ "$http_code" = "200" ]; then
        echo -e "  Status: ${GREEN}$http_code OK${NC}"
        user_id=$(echo "$body" | jq -r '.headers["user-id"] // "NOT FOUND"')
        session_id=$(echo "$body" | jq -r '.headers["session-id"] // "NOT FOUND"')
        echo -e "  user-id: ${GREEN}$user_id${NC}"
        echo -e "  session-id: ${GREEN}$session_id${NC}"
    else
        echo -e "  Status: ${RED}$http_code${NC}"
        echo "  (Session may be invalid or expired)"
    fi
else
    echo "  Skipped: Set SESSION_COOKIE env var to test authenticated requests"
    echo "  Example: SESSION_COOKIE=your-session-token ./test-header-enrichment.sh"
fi
echo ""

echo "========================================"
echo "Test Summary"
echo "========================================"
echo "The http-echo service (mendhak/http-https-echo) echoes back all"
echo "received headers, allowing verification of header enrichment."
echo ""
echo "When authenticated, auth-adapter adds these headers:"
echo "  - user-id: User identifier from session"
echo "  - session-id: Session identifier"
echo ""
echo "To test with authentication:"
echo "  1. Get a valid session token from your auth service"
echo "  2. Run: SESSION_COOKIE=<token> ./test-header-enrichment.sh"
echo "========================================"

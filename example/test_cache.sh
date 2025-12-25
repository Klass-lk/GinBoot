#!/bin/bash

BASE_URL="http://localhost:8080/api/v1"

echo "1. Creating a new post..."
curl -X POST "$BASE_URL/posts" \
  -H "Content-Type: application/json" \
  -d '{"title": "Cache Test Post", "content": "Testing caching", "author": "tester", "tags": ["cache"]}'
echo -e "\n"

echo "2. Getting posts (First Request - Expect MISS)..."
curl -v "$BASE_URL/posts" 2>&1 | grep "X-Cache"
echo -e "\n"

echo "3. Getting posts (Second Request - Expect HIT)..."
curl -v "$BASE_URL/posts" 2>&1 | grep "X-Cache"
echo -e "\n"

echo "4. Invalidating cache manually (via CacheController)..."
curl -X POST "$BASE_URL/cache/invalidate?tag=posts"
echo -e "\n"

echo "5. Getting posts (After Invalidation - Expect MISS)..."
curl -v "$BASE_URL/posts" 2>&1 | grep "X-Cache"
echo -e "\n"

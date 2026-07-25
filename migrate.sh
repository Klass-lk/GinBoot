#!/bin/bash
cd docs-site/content/docs

# Clear existing docs
rm -rf *

# 1. Getting Started
mkdir -p "1-getting-started"
cat << 'JSON' > "1-getting-started/meta.json"
{
  "title": "Getting Started",
  "pages": ["index"]
}
JSON

# We'll copy README.md to index.mdx and fix some links later if needed
cp ../../../README.md "1-getting-started/index.mdx"

# 2. Core Concepts
mkdir -p "2-core-concepts"
cat << 'JSON' > "2-core-concepts/meta.json"
{
  "title": "Core Concepts"
}
JSON

cp ../../../docs/server.md "2-core-concepts/server.mdx"
cp ../../../docs/routing.md "2-core-concepts/routing.mdx"

# 3. Features
mkdir -p "3-features"
cat << 'JSON' > "3-features/meta.json"
{
  "title": "Features"
}
JSON

cp ../../../docs/database.md "3-features/database.mdx"
cp ../../../docs/authentication.md "3-features/authentication.mdx"
cp ../../../docs/caching.md "3-features/caching.mdx"

# 4. Advanced
mkdir -p "4-advanced"
cat << 'JSON' > "4-advanced/meta.json"
{
  "title": "Advanced & Operations"
}
JSON

cp ../../../docs/telemetry.md "4-advanced/telemetry.mdx"
cp ../../../docs/testing.md "4-advanced/testing.mdx"
cp ../../../docs/deployment.md "4-advanced/deployment.mdx"

# Fix relative links in markdown files
find . -type f -name "*.mdx" -exec sed -i '' 's/\.md/\.mdx/g' {} +
find . -type f -name "*.mdx" -exec sed -i '' 's/docs\///g' {} +
find . -type f -name "*.mdx" -exec sed -i '' 's/\.\///g' {} +


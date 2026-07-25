#!/bin/bash
find . -type f -name "*.mdx" | while read file; do
  title=$(grep -m 1 "^# " "$file" | sed 's/^# //; s/"/\\"/g')
  if [ -z "$title" ]; then
    title=$(basename "$file" .mdx)
  fi
  
  # Check if frontmatter already exists
  if ! head -n 1 "$file" | grep -q "^---$"; then
    temp_file=$(mktemp)
    echo "---" > "$temp_file"
    echo "title: \"$title\"" >> "$temp_file"
    echo "---" >> "$temp_file"
    cat "$file" >> "$temp_file"
    mv "$temp_file" "$file"
  fi
done

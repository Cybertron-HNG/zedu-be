#!/usr/bin/env bash
set -euo pipefail

BASE="${1:-origin/main}"
HEAD="${2:-HEAD}"

echo "Checking changed files between ${BASE}...${HEAD}"

mapfile -t CHANGED < <(git diff --name-only --diff-filter=ACMR "${BASE}...${HEAD}")

if [ "${#CHANGED[@]}" -eq 0 ]; then
  echo "No changed files to validate."
  exit 0
fi

FORBIDDEN_PATTERNS=(
  '^\.env$'
  '^\.env\.'
  '^\.ENV$'
  '\.pem$'
  'credentials\.json$'
  'secrets\.json$'
  'service-account.*\.json$'
  '^\.aws/'
  '^\.ssh/'
)

ALLOWED_EXTENSIONS=(
  '.ts' '.tsx' '.js' '.jsx' '.mjs' '.cjs'
  '.json' '.yaml' '.yml' '.md' '.mdx'
  '.css' '.scss' '.html' '.svg' '.png' '.jpg' '.jpeg' '.gif' '.webp' '.ico'
  '.woff' '.woff2' '.ttf' '.eot'
  '.sh' '.toml' '.cjs' '.txt' '.csv'
  '.go' '.mod' '.sum' '.proto' '.sql'
)

MAX_FILE_SIZE_BYTES=$((1024 * 1024)) # 1 MB

failures=0

for file in "${CHANGED[@]}"; do
  if [ ! -e "$file" ]; then
    continue
  fi

  for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
    if [[ "$file" =~ $pattern ]]; then
      echo "ERROR: Forbidden file in PR: $file (matches /$pattern/)"
      failures=$((failures + 1))
    fi
  done

  if [ -f "$file" ]; then
    size=$(wc -c <"$file" | tr -d ' ')
    if [ "$size" -gt "$MAX_FILE_SIZE_BYTES" ]; then
      echo "ERROR: File exceeds 1MB limit: $file (${size} bytes)"
      failures=$((failures + 1))
    fi

    if file "$file" | grep -q "executable"; then
      mime=$(file -b "$file")
      if echo "$mime" | grep -Eiq 'executable|binary|archive|compressed'; then
        if [[ ! "$file" =~ \.(png|jpg|jpeg|gif|webp|ico|woff2?|ttf|eot)$ ]]; then
          ext="${file##*.}"
          allowed=false
          for allowed_ext in "${ALLOWED_EXTENSIONS[@]}"; do
            if [ ".$ext" = "$allowed_ext" ]; then
              allowed=true
              break
            fi
          done
          if [ "$allowed" = false ] && echo "$mime" | grep -qi 'binary'; then
            echo "ERROR: Unexpected binary file: $file ($mime)"
            failures=$((failures + 1))
          fi
        fi
      fi
    fi
  fi
done

if [ "$failures" -gt 0 ]; then
  echo ""
  echo "File policy check failed with $failures issue(s)."
  exit 1
fi

echo "File policy check passed for ${#CHANGED[@]} file(s)."

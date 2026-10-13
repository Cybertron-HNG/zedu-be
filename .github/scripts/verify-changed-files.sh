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
  '(^|/)\.[eE][nN][vV](\.|$)'
  '\.pem$'
  'credentials\.json$'
  'secrets\.json$'
  'service-account.*\.json$'
  '(^|/)\.aws/'
  '(^|/)\.ssh/'
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

    # MIME type, not descriptive output: ELF/Go binaries report "executable" but never "binary",
    # and archives (.zip, .tar.gz) reach neither word.
    mime=$(file -b --mime-type "$file")
    if [[ "$mime" =~ ^application/(x-executable|x-pie-executable|x-sharedlib|x-mach-binary|x-dosexec|zip|gzip|x-tar|x-7z-compressed|x-rar|octet-stream)$ ]]; then
      if [[ ! "$file" =~ \.(png|jpg|jpeg|gif|webp|ico|woff2?|ttf|eot)$ ]]; then
        echo "ERROR: Unexpected binary file: $file ($mime)"
        failures=$((failures + 1))
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

#!/usr/bin/env bash
set -eo pipefail

# Force the C locale so printf/awk emit "." decimals regardless of the user locale
export LC_ALL=C

# Root folder using git
BASE_PATH=$(git rev-parse --show-toplevel)

# The table is derived from history, so a truncated history gives wrong numbers.
if [ "$(git rev-parse --is-shallow-repository)" = "true" ]; then
    echo "update-filesize.sh needs the full git history; run 'git fetch --unshallow' first" >&2
    exit 1
fi

EXAMPLES_FOLDER="$BASE_PATH/examples"
README="$EXAMPLES_FOLDER/README.md"
# Every exported example, in the order SVG, GIF, WebM
EXAMPLE_FILES=("$EXAMPLES_FOLDER"/*.svg "$EXAMPLES_FOLDER"/*.gif "$EXAMPLES_FOLDER"/*.webm)

# Markdown table header
TABLE='| File | Iterations | First Size | Current Size | Variation |
|------|:----------:|------------|--------------|-----------|'
TABLE+=$'\n'

format_size() {
    if command -v numfmt >/dev/null 2>&1; then
        numfmt --to=si --suffix=B --format=%.2f "$1"
    else
        awk -v bytes="$1" 'BEGIN {
            split("B kB MB GB TB PB", units)
            for (unit = 1; bytes >= 1000 && unit < 6; unit++) bytes /= 1000
            printf "%.2f%s\n", bytes, units[unit]
        }'
    fi
}

for filepath in "${EXAMPLE_FILES[@]}"; do
    [ -e "$filepath" ] || continue
    filename=$(basename "$filepath")

    # Commits that changed the file, newest first. Only the checked-out
    # history counts (first-parent: changes that landed on this branch), so the
    # numbers do not depend on which other branches or refs a clone happens to have.
    blobs=$(git log --first-parent --format=%H -- "examples/$filename")

    iterations=0
    first=
    first_formatted=
    for blob in $blobs; do
        # Join commit hash with file path
        gittag="$blob:examples/$filename"

        git cat-file -e "$gittag" 2>/dev/null || continue

        # Obtain file size history in bytes
        bytes=$(git cat-file -s "$gittag")
        iterations=$((iterations + 1))
        first=$bytes

        # Format bytes to human readable sizes
        first_formatted=$(format_size "$bytes")
    done

    if [ "$iterations" -eq 0 ]; then
        echo "No readable history for examples/$filename" >&2
        exit 1
    fi

    current=$(wc -c < "$filepath" | tr -d '[:space:]')
    current_formatted=$(format_size "$current")

    # Calculate variation percent with awk (floating point, no bc dependency)
    variation=$(awk -v first="$first" -v current="$current" 'BEGIN { printf "%.4f", (first - current) / first * 100 }')

    # Append row to table
    TABLE+="| $filename | $iterations | $first_formatted | $current_formatted | $variation% |"$'\n'
done

TABLE+=$'\n'

lead='<!--SIZES_START-->'
tail='<!--SIZES_END-->'

# Replace markers with table
tmp_readme=$(mktemp "$README.XXXXXX")
tmp_table=
trap 'rm -f "$tmp_readme"; [ -z "$tmp_table" ] || rm -f "$tmp_table"' EXIT
tmp_table=$(mktemp "$README.table.XXXXXX")
printf '%s' "$TABLE" > "$tmp_table"
awk -v lead="$lead" -v tail="$tail" -v table_file="$tmp_table" '
    $0 == lead {
        lead_count++
        if (lead_count != 1 || in_sizes || tail_count) invalid = 1
        in_sizes = 1
        print
        while ((getline line < table_file) > 0) print line
        close(table_file)
        next
    }
    $0 == tail {
        tail_count++
        if (!in_sizes || tail_count != 1) invalid = 1
        in_sizes = 0
    }
    !in_sizes { print }
    END {
        if (invalid || lead_count != 1 || tail_count != 1 || in_sizes) exit 1
    }
' "$README" > "$tmp_readme" || {
    echo "README size markers must be one ordered pair" >&2
    exit 1
}
mv "$tmp_readme" "$README"

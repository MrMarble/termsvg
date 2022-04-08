#!/usr/bin/env bash

# Root folder using git
BASE_PATH=$(git rev-parse --show-toplevel)

EXAMPLES_FOLDER="$BASE_PATH/examples"
SVG_FILES="$EXAMPLES_FOLDER/*.svg"

# Markdown table header
read -r -d '' TABLE << EOS
| File | Iterations | First Size | Current Size | Variation |
|------|:----------:|------------|--------------|-----------|\n
EOS


for filepath in $SVG_FILES; do
    filename=$(basename $filepath)

    # Get git commit hash history for file
    blobs=$(git rev-list --all examples/$filename)

    formated_sizes=()
    raw_sizes=()
    for blob in $blobs; do
        # Join commit hash with file path
        gittag="$blob:examples/$filename"

        # Obtain file size history in bytes
        bytes=$(git cat-file -s $gittag)
        raw_sizes+=($bytes)

        # Format bytes to human readable sizes
        formated_sizes+=($(numfmt --to=si --suffix=B --format=%.2f $bytes))
    done

    first=${raw_sizes[-1]}
    current=${raw_sizes[0]}

    # Calculate variation percent using bc to suport floating point
    variation=`echo "scale=4; (($first-$current)/(($first+$current)/2))*100" | bc`

    # Append row to table
    TABLE+="| $filename | ${#formated_sizes[@]} | ${formated_sizes[-1]} | ${formated_sizes[0]} | $variation% |\n"
done

lead='<!--SIZES_START-->'
tail='<!--SIZES_END-->'

# Replace markers with table
new_readme=$(sed -n "/$lead/{p;:a;N;/$tail/!ba;s/.*\n/${TABLE//$'\n'/\\n}\n/};p" $EXAMPLES_FOLDER/README.md)
echo "$new_readme" > $EXAMPLES_FOLDER/README.md
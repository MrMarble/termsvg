#!/usr/bin/env bash
set -euo pipefail

BASE_PATH=$(git rev-parse --show-toplevel)
TERMSVG=${TERMSVG:-"$BASE_PATH/termsvg"}

if [[ ! -x "$TERMSVG" ]]; then
    echo "termsvg binary not found at $TERMSVG; run task build first" >&2
    exit 1
fi

if (($#)); then
    fixtures=("$@")
else
    fixtures=(
        "$BASE_PATH/examples/444816.cast"
        "$BASE_PATH/examples/444816.cast:borderless"
        "$BASE_PATH/examples/htop.cast"
        "$BASE_PATH/examples/session.cast"
        "$BASE_PATH/examples/256colors.cast"
        "$BASE_PATH/examples/rgb.cast"
    )
fi

TMP=$(mktemp -d "${TMPDIR:-/tmp}/termsvg-svg-matrix.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

raw_files=()
metric_args=()
render_times=()

clock_ns() {
    python3 -c 'import time; print(time.time_ns())'
}

render_variant() {
    local fixture_name=$1 cast=$2 borderless=$3 fps_name=$4 max_fps=$5 variant=$6
    shift 6
    local name="$fixture_name-$fps_name-$variant"
    local raw="$TMP/$name.raw.svg"
    local minified="$TMP/$name.min.svg"
    local args=(--svg-max-fps="$max_fps")
    if [[ $borderless == true ]]; then
        args+=(-n)
    fi
    args+=("$@")

    local started finished
    started=$(clock_ns)
    "$TERMSVG" export "$cast" "${args[@]}" -o "$raw" >/dev/null
    finished=$(clock_ns)
    "$TERMSVG" export "$cast" "${args[@]}" -m -o "$minified" >/dev/null

    raw_files+=("$raw")
    metric_args+=("-minified" "$raw=$minified")
    render_times+=("$((finished-started))")
}

for fixture in "${fixtures[@]}"; do
    borderless=false
    cast=$fixture
    if [[ $fixture == *:borderless ]]; then
        borderless=true
        cast=${fixture%:borderless}
    fi
    if [[ ! -f "$cast" ]]; then
        echo "cast file not found: $cast" >&2
        exit 1
    fi
    fixture_name=$(basename "$cast" .cast)
    if [[ $borderless == true ]]; then
        fixture_name+="-borderless"
    fi

    for fps in 0 30; do
        fps_name=lossless
        if ((fps)); then
            fps_name=30fps
        fi
        render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" frames-css-translate
        render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" frames-smil-translate --svg-animation=smil
        render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" frames-smil-href --svg-animation=smil --svg-frame-switch=href
        render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" bands-css-translate --svg-layout=bands
        render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" bands-smil-translate --svg-layout=bands --svg-animation=smil
        render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" bands-smil-href --svg-layout=bands --svg-animation=smil --svg-frame-switch=href
        render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" regions-css-translate --svg-layout=regions
        render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" regions-smil-translate --svg-layout=regions --svg-animation=smil
        render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" regions-smil-href --svg-layout=regions --svg-animation=smil --svg-frame-switch=href
    done
done

printf '# fixtures=%s fps=lossless,30\n' "${fixtures[*]}"
printf '%s\n' "${render_times[@]}" > "$TMP/render-times"
go run ./scripts/svgmetrics "${metric_args[@]}" "${raw_files[@]}" |
    awk -F '\t' -v OFS='\t' -v times="$TMP/render-times" '
        NR == 1 { print "variant", "render_ns", $0; next }
        {
            getline elapsed < times
            file = $1
            sub(/^.*\//, "", file)
            sub(/\.raw\.svg$/, "", file)
            print file, elapsed, $0
        }
    '

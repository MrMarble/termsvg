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

if [[ -z ${EPOCHREALTIME:-} ]]; then
    echo "bash 5 or newer is required (EPOCHREALTIME)" >&2
    exit 1
fi

# clock_ns prints the wall clock in nanoseconds, at microsecond resolution.
# EPOCHREALTIME is seconds and microseconds; its separator depends on the locale.
clock_ns() {
    local now=${EPOCHREALTIME/[.,]/}
    echo "$((now * 1000))"
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

    variant() { render_variant "$fixture_name" "$cast" "$borderless" "$fps_name" "$fps" "$@"; }

    for fps in 0 30; do
        fps_name=lossless
        if ((fps)); then
            fps_name=30fps
        fi
        variant frames-css-translate
        variant frames-smil-translate --svg-animation=smil
        variant frames-smil-href --svg-animation=smil --svg-frame-switch=href
        variant bands-css-translate --svg-layout=bands
        variant bands-smil-translate --svg-layout=bands --svg-animation=smil
        variant bands-smil-href --svg-layout=bands --svg-animation=smil --svg-frame-switch=href
        variant regions-css-translate --svg-layout=regions
        variant regions-smil-translate --svg-layout=regions --svg-animation=smil
        variant regions-smil-href --svg-layout=regions --svg-animation=smil --svg-frame-switch=href
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

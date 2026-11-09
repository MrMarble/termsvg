package svg

import "strings"

// useMarkup is the one builder of <use href> references to shared definitions;
// dx and dy are the optional positional offsets.
func useMarkup(minify bool, id string, dx, dy int) string {
	var sb strings.Builder
	sb.WriteString(`<use href="#`)
	sb.WriteString(id)
	sb.WriteByte('"')
	if dx != 0 {
		sb.WriteString(` x="`)
		sb.WriteString(formatInt(dx, minify))
		sb.WriteByte('"')
	}
	if dy != 0 {
		sb.WriteString(` y="`)
		sb.WriteString(formatInt(dy, minify))
		sb.WriteByte('"')
	}
	sb.WriteString(`/>`)
	return sb.String()
}

func useBytes(minify bool, idLen, dx, dy int) int {
	bytes := len(`<use href="#`) + idLen + len(`"/>`)
	if dx != 0 {
		bytes += len(` x=""`) + len(formatInt(dx, minify))
	}
	if dy != 0 {
		bytes += len(` y=""`) + len(formatInt(dy, minify))
	}
	return bytes
}

// rowReferenceBytes is the serialized size of a whole-row <use> reference with
// a two-letter id.
var rowReferenceBytes = useBytes(false, 2, 0, 0)

// Command svgmetrics reports deterministic size and structure metrics for SVG files.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

type metrics struct {
	RawBytes, MinifiedBytes, GzipBytes, BrotliBytes    int
	Elements, FilterRefs, Keyframes, KeyframeSelectors int
	DuplicateSelectors, AnimatedGroups                 int
	DefinitionNodes, ActiveNodes                       int
	TextNodes, RectNodes, GroupNodes, UseNodes         int
	AnimationNodes, AnimatedElements, StateDefinitions int
	MaxUseDepth, LocalViewportCount                    int
	MaxViewportWidth, MaxViewportHeight                int
	MaxTranslatedWidth, MaxTranslatedArea              int64
	MaxTranslate                                       float64
	Tags                                               map[string]int
}

type viewport struct{ width, height int }

type cssToken struct {
	typeID css.TokenType
	text   string
	spaced bool
}

type cssMetrics struct {
	filters, keyframes, selectors, duplicates int
	animatedSelectors                         [][]string
}

type groupInfo struct {
	classes         []string
	inlineAnimation bool
	smilAnimation   bool
}

type declarationMetrics struct {
	filters    int
	animated   bool
	transforms []string
}

type pairs map[string]string

var (
	translateRE = regexp.MustCompile(`(?i)translate(x|y|3d)?\(([^)]*)\)`)
	numberRE    = regexp.MustCompile(`[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?`)
)

func measure(raw, minified []byte) (metrics, error) {
	result := metrics{RawBytes: len(raw), MinifiedBytes: len(minified), Tags: map[string]int{}}
	if err := validXML(minified); err != nil {
		return result, fmt.Errorf("minified SVG: %w", err)
	}
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(minified); err != nil {
		return result, err
	}
	if err := zw.Close(); err != nil {
		return result, err
	}
	result.GzipBytes = compressed.Len()
	result.BrotliBytes = brotliSize(minified)

	styles, transforms, groups, err := scanSVG(raw, &result)
	if err != nil {
		return result, err
	}
	selectors, err := addStyleMetrics(styles, &result, &transforms)
	if err != nil {
		return result, err
	}
	result.AnimatedGroups = countAnimatedGroups(groups, selectors)
	maximum, err := maxTranslate(transforms)
	if err != nil {
		return result, err
	}
	result.MaxTranslate = maximum
	if len(raw) > 0 && result.MaxTranslatedWidth == 0 && maximum > 0 {
		result.MaxTranslatedWidth = int64(maximum)
	}
	return result, nil
}

//nolint:gocognit,funlen // Stateful XML scan keeps parent, group, style, and transform state synchronized.
func scanSVG(raw []byte, result *metrics) (styles, transforms []string, groups []groupInfo, err error) {
	var parents []int
	var owners []string
	var viewports []viewport
	var elementIDs []int
	nextElementID := 0
	defsDepth := 0
	rootSVGSeen := false
	animatedParents := map[int]bool{}
	definitionUses := map[string][]string{}
	stateTargets := map[string]bool{}
	var activeUses []string
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			name := token.Name.Local
			id, href := "", ""
			currentViewport := viewport{}
			if len(viewports) > 0 {
				currentViewport = viewports[len(viewports)-1]
			}
			var elementTransforms []string
			attributeName, values := "", ""
			for _, attr := range token.Attr {
				switch attr.Name.Local {
				case "id":
					id = attr.Value
				case "href":
					href = strings.TrimPrefix(attr.Value, "#")
				case "width":
					currentViewport.width = dimension(attr.Value)
				case "height":
					currentViewport.height = dimension(attr.Value)
				case "transform":
					elementTransforms = append(elementTransforms, attr.Value)
				case "attributeName":
					attributeName = attr.Value
				case "values":
					values = attr.Value
				}
			}
			if name == "animate" && attributeName == "href" {
				// State definitions are the targets an href animation switches between.
				for _, target := range strings.Split(values, ";") {
					if target = strings.TrimPrefix(strings.TrimSpace(target), "#"); target != "" {
						stateTargets[target] = true
					}
				}
			}
			if name == "svg" {
				if rootSVGSeen {
					result.LocalViewportCount++
					result.MaxViewportWidth = max(result.MaxViewportWidth, currentViewport.width)
					result.MaxViewportHeight = max(result.MaxViewportHeight, currentViewport.height)
				}
				rootSVGSeen = true
			}
			definition := defsDepth > 0 || name == "defs"
			result.Elements++
			result.Tags[name]++
			if definition {
				result.DefinitionNodes++
			} else {
				result.ActiveNodes++
			}
			switch name {
			case "text":
				result.TextNodes++
			case "rect":
				result.RectNodes++
			case "g":
				result.GroupNodes++
			case "use":
				result.UseNodes++
			case "animate", "animateTransform", "animateMotion":
				result.AnimationNodes++
				if len(elementIDs) > 0 {
					animatedParents[elementIDs[len(elementIDs)-1]] = true
				}
			}
			owner := ""
			if len(owners) > 0 {
				owner = owners[len(owners)-1]
			}
			if defsDepth > 0 && id != "" {
				owner = id
			}
			if name == "use" && href != "" {
				if defsDepth > 0 && owner != "" {
					definitionUses[owner] = append(definitionUses[owner], href)
				} else {
					activeUses = append(activeUses, href)
				}
			}
			owners = append(owners, owner)
			viewports = append(viewports, currentViewport)
			nextElementID++
			elementIDs = append(elementIDs, nextElementID)
			for _, transform := range elementTransforms {
				addTranslatedMetrics(result, transform, currentViewport)
			}
			parent := -1
			if len(parents) > 0 {
				parent = parents[len(parents)-1]
			}
			if name == "g" {
				parent = len(groups)
				groups = append(groups, groupInfo{})
			}
			parents = append(parents, parent)
			for _, attr := range token.Attr {
				switch attr.Name.Local {
				case "filter":
					result.FilterRefs++
				case "class":
					if name == "g" {
						groups[parent].classes = strings.Fields(attr.Value)
					}
				case "style":
					analysis, err := analyzeDeclarations(attr.Value)
					if err != nil {
						return nil, nil, nil, err
					}
					result.FilterRefs += analysis.filters
					if name == "g" && analysis.animated {
						groups[parent].inlineAnimation = true
					}
					transforms = append(transforms, analysis.transforms...)
				case "transform":
					transforms = append(transforms, attr.Value)
				}
			}
			if parent >= 0 && isSMILAnimation(name) {
				groups[parent].smilAnimation = true
			}
			if name == "defs" {
				defsDepth++
			}
			if name == "style" {
				var text string
				if err := decoder.DecodeElement(&text, &token); err != nil {
					return nil, nil, nil, err
				}
				parents = parents[:len(parents)-1]
				owners = owners[:len(owners)-1]
				viewports = viewports[:len(viewports)-1]
				elementIDs = elementIDs[:len(elementIDs)-1]
				styles = append(styles, text)
			}
		case xml.EndElement:
			if token.Name.Local == "defs" {
				defsDepth--
			}
			parents = parents[:len(parents)-1]
			owners = owners[:len(owners)-1]
			viewports = viewports[:len(viewports)-1]
			elementIDs = elementIDs[:len(elementIDs)-1]
		}
	}
	result.AnimatedElements = len(animatedParents)
	result.StateDefinitions = len(stateTargets)
	result.MaxUseDepth = maxUseDepth(activeUses, definitionUses)
	return styles, transforms, groups, nil
}

func dimension(value string) int {
	number := numberRE.FindString(value)
	parsed, _ := strconv.ParseFloat(number, 64)
	return int(parsed)
}

func addTranslatedMetrics(result *metrics, transform string, viewport viewport) {
	distance, err := maxTranslate([]string{transform})
	if err != nil || distance == 0 {
		return
	}
	width := int64(distance) + int64(viewport.width)
	result.MaxTranslatedWidth = max(result.MaxTranslatedWidth, width)
	result.MaxTranslatedArea = max(result.MaxTranslatedArea, width*int64(viewport.height))
}

func maxUseDepth(active []string, definitions map[string][]string) int {
	var visit func(string, map[string]bool) int
	visit = func(id string, seen map[string]bool) int {
		if seen[id] {
			return 0
		}
		seen[id] = true
		depth := 1
		for _, child := range definitions[id] {
			depth = max(depth, 1+visit(child, seen))
		}
		delete(seen, id)
		return depth
	}
	depth := 0
	for _, id := range active {
		depth = max(depth, visit(id, map[string]bool{}))
	}
	return depth
}

func brotliSize(data []byte) int {
	path, err := exec.LookPath("brotli")
	if err != nil {
		return -1
	}
	command := exec.Command(path, "-q", "11", "-c") //nolint:gosec // fixed command, explicit local data
	command.Stdin = bytes.NewReader(data)
	output, err := command.Output()
	if err != nil {
		return -1
	}
	return len(output)
}

func addStyleMetrics(styles []string, result *metrics, transforms *[]string) ([][]string, error) {
	var selectors [][]string
	for _, text := range styles {
		m, err := analyzeCSS(text)
		if err != nil {
			return nil, err
		}
		result.FilterRefs += m.filters
		result.Keyframes += m.keyframes
		result.KeyframeSelectors += m.selectors
		result.DuplicateSelectors += m.duplicates
		selectors = append(selectors, m.animatedSelectors...)
		values, err := cssTransforms(text)
		if err != nil {
			return nil, err
		}
		*transforms = append(*transforms, values...)
	}
	return selectors, nil
}

func countAnimatedGroups(groups []groupInfo, selectors [][]string) int {
	count := 0
	for _, group := range groups {
		animated := group.inlineAnimation || group.smilAnimation
		classes := map[string]bool{}
		for _, class := range group.classes {
			classes[class] = true
		}
		for _, selector := range selectors {
			matches := true
			for _, class := range selector {
				matches = matches && classes[class]
			}
			animated = animated || matches
		}
		if animated {
			count++
		}
	}
	return count
}

func maxTranslate(transforms []string) (float64, error) {
	var maximum float64
	for _, text := range transforms {
		for _, call := range translateRE.FindAllStringSubmatch(text, -1) {
			if strings.EqualFold(call[1], "y") {
				continue
			}
			number := numberRE.FindString(call[2])
			value, err := strconv.ParseFloat(number, 64)
			if err != nil {
				return 0, err
			}
			if value < 0 {
				value = -value
			}
			if value > maximum {
				maximum = value
			}
		}
	}
	return maximum, nil
}

func validXML(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := decoder.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func lexCSS(text string) ([]cssToken, error) {
	lexer := css.NewLexer(parse.NewInputString(text))
	var tokens []cssToken
	depth := 0
	spaced := false
	for {
		typeID, data := lexer.Next()
		if typeID == css.ErrorToken {
			if errors.Is(lexer.Err(), io.EOF) {
				break
			}
			return nil, lexer.Err()
		}
		if typeID == css.WhitespaceToken {
			spaced = true
			continue
		}
		if typeID == css.CommentToken {
			continue
		}
		if typeID == css.LeftBraceToken {
			depth++
		}
		if typeID == css.RightBraceToken {
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced CSS braces")
			}
		}
		tokens = append(tokens, cssToken{typeID: typeID, text: string(data), spaced: spaced})
		spaced = false
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced CSS braces")
	}
	return tokens, nil
}

func isSMILAnimation(name string) bool {
	return name == "animate" || name == "animateTransform" || name == "animateMotion"
}

//nolint:gocognit,funlen // Stateful CSS token traversal tracks nested keyframes and rules.
func analyzeCSS(text string) (cssMetrics, error) {
	tokens, err := lexCSS(text)
	if err != nil {
		return cssMetrics{}, err
	}
	var result cssMetrics
	for i := 0; i < len(tokens); i++ {
		if i+1 < len(tokens) && tokens[i].typeID == css.IdentToken &&
			strings.EqualFold(tokens[i].text, "filter") && tokens[i+1].typeID == css.ColonToken {
			result.filters++
		}
		if tokens[i].typeID != css.AtKeywordToken || !strings.EqualFold(tokens[i].text, "@keyframes") {
			continue
		}
		result.keyframes++
		for i < len(tokens) && tokens[i].typeID != css.LeftBraceToken {
			i++
		}
		if i == len(tokens) {
			return result, fmt.Errorf("malformed @keyframes")
		}
		depth, seen := 1, map[string]bool{}
		for i++; i < len(tokens) && depth > 0; i++ {
			token := tokens[i]
			if depth == 1 {
				selector := ""
				//nolint:exhaustive // Only CSS percentage and from/to tokens are selectors here.
				switch token.typeID {
				case css.PercentageToken:
					value, err := strconv.ParseFloat(strings.TrimSuffix(token.text, "%"), 64)
					if err != nil {
						return result, err
					}
					selector = strconv.FormatFloat(value, 'g', -1, 64) + "%"
				case css.IdentToken:
					switch {
					case strings.EqualFold(token.text, "from"):
						selector = "0%"
					case strings.EqualFold(token.text, "to"):
						selector = "100%"
					}
				default:
				}
				if selector != "" {
					result.selectors++
					if seen[selector] {
						result.duplicates++
					}
					seen[selector] = true
				}
			}
			//nolint:exhaustive // Only braces change the keyframe nesting depth.
			switch token.typeID {
			case css.LeftBraceToken:
				depth++
			case css.RightBraceToken:
				depth--
			default:
			}
		}
		i--
	}

	for start := 0; start < len(tokens); {
		open := start
		for open < len(tokens) && tokens[open].typeID != css.LeftBraceToken {
			open++
		}
		if open == len(tokens) {
			break
		}
		depth, end := 1, open+1
		for ; end < len(tokens) && depth > 0; end++ {
			//nolint:exhaustive // Only braces change the rule nesting depth.
			switch tokens[end].typeID {
			case css.LeftBraceToken:
				depth++
			case css.RightBraceToken:
				depth--
			default:
			}
		}
		_, animated := activeAnimation(tokens[open+1 : end-1])
		if animated {
			result.animatedSelectors = append(result.animatedSelectors, simpleClassSelectors(tokens[start:open])...)
		}
		start = end
	}
	return result, nil
}

func analyzeDeclarations(text string) (declarationMetrics, error) {
	tokens, err := lexCSS(text)
	if err != nil {
		return declarationMetrics{}, err
	}
	var result declarationMetrics
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].typeID != css.IdentToken || tokens[i+1].typeID != css.ColonToken {
			continue
		}
		name := strings.ToLower(tokens[i].text)
		if name == "filter" {
			result.filters++
		}
	}
	_, result.animated = activeAnimation(tokens)
	transforms, err := cssTransforms(text)
	result.transforms = transforms
	return result, err
}

//nolint:lll // CSS animation property spellings are clearer as direct comparisons.
func isAnimationProperty(name string) bool {
	name = strings.ToLower(name)
	return name == "animation" || name == "animation-name" || name == "-webkit-animation" || name == "-webkit-animation-name"
}

//nolint:lll // Token boundary checks mirror CSS declaration syntax.
func activeAnimation(tokens []cssToken) (seen, active bool) {
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].typeID != css.IdentToken || tokens[i+1].typeID != css.ColonToken || !isAnimationProperty(tokens[i].text) {
			continue
		}
		seen = true
		active = false
		for i += 2; i < len(tokens) && tokens[i].typeID != css.SemicolonToken && tokens[i].typeID != css.RightBraceToken; i++ {
			if tokens[i].typeID == css.IdentToken && animationName(strings.ToLower(tokens[i].text)) {
				active = true
			}
		}
	}
	return seen, active
}

//nolint:lll // CSS animation keywords are clearer as a single exhaustive list.
func animationName(name string) bool {
	switch name {
	case "none", "initial", "inherit", "unset", "revert", "revert-layer", "infinite", "linear", "ease", "ease-in", "ease-out", "ease-in-out", "step-start", "step-end", "running", "paused", "normal", "reverse", "alternate", "alternate-reverse", "forwards", "backwards", "both":
		return false
	default:
		return true
	}
}

//nolint:lll // Selector validation mirrors the compact token-pair grammar.
func simpleClassSelectors(tokens []cssToken) [][]string {
	var selectors [][]string
	for start := 0; start < len(tokens); {
		end := start
		for end < len(tokens) && tokens[end].typeID != css.CommaToken {
			end++
		}
		var classes []string
		valid := start < end
		for i := start; valid && i < end; i += 2 {
			valid = i+1 < end && tokens[i].typeID == css.DelimToken && tokens[i].text == "." && (i == start || !tokens[i].spaced) && tokens[i+1].typeID == css.IdentToken && !tokens[i+1].spaced
			if valid {
				classes = append(classes, tokens[i+1].text)
			}
		}
		if valid {
			selectors = append(selectors, classes)
		}
		start = end + 1
	}
	return selectors
}

//nolint:lll // Token boundary checks mirror CSS declaration syntax.
func cssTransforms(text string) ([]string, error) {
	tokens, err := lexCSS(text)
	if err != nil {
		return nil, err
	}
	var values []string
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].typeID != css.IdentToken || !strings.EqualFold(tokens[i].text, "transform") || tokens[i+1].typeID != css.ColonToken {
			continue
		}
		var value strings.Builder
		for i += 2; i < len(tokens) && tokens[i].typeID != css.SemicolonToken && tokens[i].typeID != css.RightBraceToken; i++ {
			if tokens[i].typeID != css.StringToken {
				value.WriteString(tokens[i].text)
			}
		}
		values = append(values, value.String())
	}
	return values, nil
}

func (p pairs) String() string { return "raw.svg=minified.svg" }
func (p pairs) Set(value string) error {
	raw, minified, ok := strings.Cut(value, "=")
	if !ok || raw == "" || minified == "" {
		return fmt.Errorf("minified pair must be raw.svg=minified.svg")
	}
	p[raw] = minified
	return nil
}

//nolint:funlen,lll // TSV schema and record order are intentionally explicit and stable.
func run(args []string, stdout io.Writer) error {
	set := flag.NewFlagSet("svgmetrics", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	minifiedFiles := pairs{}
	set.Var(minifiedFiles, "minified", "raw.svg=minified.svg (repeatable)")
	if err := set.Parse(args); err != nil {
		return err
	}
	if set.NArg() == 0 {
		return fmt.Errorf("usage: svgmetrics -minified raw.svg=minified.svg raw.svg [...]")
	}
	records := [][]string{{"file", "minified_file", "raw_bytes", "minified_bytes", "gzip_bytes", "brotli_bytes", "xml_nodes", "definition_nodes", "active_nodes", "text_nodes", "rect_nodes", "group_nodes", "use_nodes", "animation_nodes", "animated_elements", "state_definitions", "max_use_depth", "max_translated_width", "max_translated_area", "local_viewports", "max_viewport_width", "max_viewport_height", "tags", "filter_references", "keyframes", "keyframe_selectors", "duplicate_selectors", "max_translate", "animated_groups"}}
	for _, name := range set.Args() {
		minifiedName, ok := minifiedFiles[name]
		if !ok {
			return fmt.Errorf("%s: missing -minified pair", name)
		}
		//nolint:gosec // CLI accepts explicit SVG paths supplied by the caller.
		raw, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		//nolint:gosec // CLI accepts the explicit paired minified SVG path.
		minified, err := os.ReadFile(minifiedName)
		if err != nil {
			return err
		}
		m, err := measure(raw, minified)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		keys := make([]string, 0, len(m.Tags))
		for tag := range m.Tags {
			keys = append(keys, tag)
		}
		sort.Strings(keys)
		counts := make([]string, 0, len(keys))
		for _, tag := range keys {
			counts = append(counts, fmt.Sprintf("%s=%d", tag, m.Tags[tag]))
		}
		records = append(records, []string{name, minifiedName, strconv.Itoa(m.RawBytes), strconv.Itoa(m.MinifiedBytes), strconv.Itoa(m.GzipBytes), strconv.Itoa(m.BrotliBytes), strconv.Itoa(m.Elements), strconv.Itoa(m.DefinitionNodes), strconv.Itoa(m.ActiveNodes), strconv.Itoa(m.TextNodes), strconv.Itoa(m.RectNodes), strconv.Itoa(m.GroupNodes), strconv.Itoa(m.UseNodes), strconv.Itoa(m.AnimationNodes), strconv.Itoa(m.AnimatedElements), strconv.Itoa(m.StateDefinitions), strconv.Itoa(m.MaxUseDepth), strconv.FormatInt(m.MaxTranslatedWidth, 10), strconv.FormatInt(m.MaxTranslatedArea, 10), strconv.Itoa(m.LocalViewportCount), strconv.Itoa(m.MaxViewportWidth), strconv.Itoa(m.MaxViewportHeight), strings.Join(counts, ","), strconv.Itoa(m.FilterRefs), strconv.Itoa(m.Keyframes), strconv.Itoa(m.KeyframeSelectors), strconv.Itoa(m.DuplicateSelectors), strconv.FormatFloat(m.MaxTranslate, 'g', -1, 64), strconv.Itoa(m.AnimatedGroups)})
	}
	var output bytes.Buffer
	w := csv.NewWriter(&output)
	w.Comma = '\t'
	for _, record := range records {
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	_, err := stdout.Write(output.Bytes())
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

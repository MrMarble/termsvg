package css

import (
	"fmt"
	"sort"
	"strings"
)

type IRules interface {
	String() string
}

type Blocks []Block

type Block struct {
	Selector string
	Rules    IRules
}

type Rules map[string]string

func (c Rules) String() string {
	var compiled []string

	for property, value := range c {
		compiled = append(compiled, fmt.Sprintf("%s:%s", property, value))
	}

	sort.Strings(compiled)

	return strings.Join(compiled, ";")
}

func (b Block) String() string {
	return fmt.Sprintf("%s{%s}", b.Selector, b.Rules)
}

func (b Blocks) String() string {
	result := ""
	for _, block := range b {
		result += block.String()
	}

	return result
}

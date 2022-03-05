package css

import (
	"fmt"
	"strings"
)

type CSS map[string]string

func (c *CSS) Compile() string {
	var compiled []string

	for property, value := range *c {
		compiled = append(compiled, fmt.Sprintf("%s:%s", property, value))
	}

	return strings.Join(compiled, ";")
}

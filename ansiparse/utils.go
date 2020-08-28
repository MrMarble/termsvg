package ansiparse

import (
	"strings"
)

func sliceUniq(s []string) []string {
	for i := 0; i < len(s); i++ {
		for i2 := i + 1; i2 < len(s); i2++ {
			if s[i] == s[i2] {
				// delete
				s = append(s[:i2], s[i2+1:]...)
				i2--
			}
		}
	}
	return s
}

func splitString(str, delimiter string) []string {
	result := make([]string, 0)
	for _, chunk := range strings.Split(str, delimiter) {
		result = append(result, chunk, delimiter)
	}
	return result[:len(result)-1]
}

func splitSlice(slice []string, delimiter string) []string {
	result := make([]string, 0)
	for _, str := range slice {
		result = append(result, splitString(str, delimiter)...)
	}
	return result
}

func superSplit(str interface{}, delimiters []string) []string {
	if len(delimiters) == 0 {
		return str.([]string)
	}

	if str, ok := str.(string); ok {
		delimiter := delimiters[len(delimiters)-1]
		split := splitString(str, delimiter)
		return superSplit(split, delimiters[:len(delimiters)-1])
	}

	if slice, ok := str.([]string); ok {
		delimiter := delimiters[len(delimiters)-1]
		split := splitSlice(slice, delimiter)
		return superSplit(split, delimiters[:len(delimiters)-1])
	}
	return nil
}

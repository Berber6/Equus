package agent

import (
	"path/filepath"
	"strings"
)

// matchGlob reports whether the slash-separated relative path matches the
// slash-separated pattern. Unlike filepath.Match it supports '**' segments,
// which match zero or more path segments (e.g. "**/*.go", "internal/**/*.go").
func matchGlob(pattern, name string) bool {
	patternSegments := strings.Split(pattern, "/")
	nameSegments := strings.Split(name, "/")
	return matchSegments(patternSegments, nameSegments)
}

func matchSegments(pattern, name []string) bool {
	for len(pattern) > 0 {
		segment := pattern[0]
		if segment == "**" {
			// '**' matches zero or more segments: try every split.
			if len(pattern) == 1 {
				return true
			}
			for skip := 0; skip <= len(name); skip++ {
				if matchSegments(pattern[1:], name[skip:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if !matchSegment(segment, name[0]) {
			return false
		}
		pattern = pattern[1:]
		name = name[1:]
	}
	return len(name) == 0
}

// matchSegment matches a single path segment, tolerating character classes and
// '*' via filepath.Match.
func matchSegment(pattern, name string) bool {
	matched, err := filepath.Match(pattern, name)
	if err != nil {
		return pattern == name
	}
	return matched
}

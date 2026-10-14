package skill

import (
	"path/filepath"
	"sort"
)

// Discover loads skills from the user-level (<equusHome>/skills) and
// project-level (<cwd>/.equus/skills) directories. On a name clash the project
// skill wins. The result is sorted by name. Missing directories are ignored.
func Discover(equusHome, cwd string) []Skill {
	byName := make(map[string]Skill)
	for _, skill := range loadDir(filepath.Join(equusHome, "skills")) {
		byName[skill.Name] = skill
	}
	// Project-level overrides user-level.
	for _, skill := range loadDir(filepath.Join(cwd, ".equus", "skills")) {
		byName[skill.Name] = skill
	}
	skills := make([]Skill, 0, len(byName))
	for _, skill := range byName {
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills
}

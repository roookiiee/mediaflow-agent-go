package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Skill struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Path    string `json:"path"`
	Body    string `json:"body,omitempty"`
}

type Manager struct {
	skills []Skill
	byName map[string]Skill
}

func NewManager(dir string) (*Manager, error) {
	manager := &Manager{byName: map[string]Skill{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return manager, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		skill := parseSkill(path, string(body))
		manager.skills = append(manager.skills, skill)
		manager.byName[strings.ToLower(skill.Name)] = skill
	}
	sort.Slice(manager.skills, func(i, j int) bool {
		return manager.skills[i].Name < manager.skills[j].Name
	})
	return manager, nil
}

func (m *Manager) List() []Skill {
	out := make([]Skill, len(m.skills))
	copy(out, m.skills)
	for i := range out {
		out[i].Body = ""
	}
	return out
}

func (m *Manager) Load(name string) (Skill, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	skill, ok := m.byName[name]
	if ok {
		return skill, true
	}
	for key, candidate := range m.byName {
		if strings.Contains(key, name) || strings.Contains(name, key) {
			return candidate, true
		}
	}
	return Skill{}, false
}

func (m *Manager) Match(query string, limit int) []Skill {
	type scored struct {
		skill Skill
		score int
	}
	tokens := tokenSet(query)
	scoredSkills := make([]scored, 0, len(m.skills))
	for _, skill := range m.skills {
		haystack := strings.ToLower(skill.Name + " " + skill.Summary + " " + skill.Body)
		score := 0
		for token := range tokens {
			if strings.Contains(haystack, token) {
				score++
			}
		}
		if score > 0 {
			scoredSkills = append(scoredSkills, scored{skill: skill, score: score})
		}
	}
	sort.Slice(scoredSkills, func(i, j int) bool {
		if scoredSkills[i].score == scoredSkills[j].score {
			return scoredSkills[i].skill.Name < scoredSkills[j].skill.Name
		}
		return scoredSkills[i].score > scoredSkills[j].score
	})
	if limit <= 0 || limit > len(scoredSkills) {
		limit = len(scoredSkills)
	}
	out := make([]Skill, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, scoredSkills[i].skill)
	}
	return out
}

func parseSkill(path, raw string) Skill {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	summary := ""
	body := raw

	if strings.HasPrefix(raw, "---") {
		parts := strings.SplitN(raw, "---", 3)
		if len(parts) == 3 {
			meta := parts[1]
			body = strings.TrimSpace(parts[2])
			for _, line := range strings.Split(meta, "\n") {
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				switch strings.TrimSpace(strings.ToLower(key)) {
				case "name":
					name = strings.Trim(strings.TrimSpace(value), "\"'")
				case "summary":
					summary = strings.Trim(strings.TrimSpace(value), "\"'")
				}
			}
		}
	}

	if summary == "" {
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if line != "" {
				summary = line
				break
			}
		}
	}

	return Skill{Name: name, Summary: summary, Path: path, Body: body}
}

func tokenSet(text string) map[string]struct{} {
	tokens := map[string]struct{}{}
	var builder strings.Builder
	flush := func() {
		if builder.Len() < 2 {
			builder.Reset()
			return
		}
		tokens[strings.ToLower(builder.String())] = struct{}{}
		builder.Reset()
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return tokens
}

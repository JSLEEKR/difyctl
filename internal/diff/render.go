package diff

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// RenderText prints a categorized, human-readable diff to w.
func RenderText(w io.Writer, changes []Change) {
	if len(changes) == 0 {
		fmt.Fprintln(w, "no semantic changes")
		return
	}
	groups := map[string][]Change{}
	for _, c := range changes {
		groups[c.Category] = append(groups[c.Category], c)
	}
	order := []string{CategoryBreaking, CategoryRemoved, CategoryAdded, CategoryChanged}
	for _, cat := range order {
		cs := groups[cat]
		if len(cs) == 0 {
			continue
		}
		sort.SliceStable(cs, func(i, j int) bool {
			if cs[i].Kind != cs[j].Kind {
				return cs[i].Kind < cs[j].Kind
			}
			if cs[i].ID != cs[j].ID {
				return cs[i].ID < cs[j].ID
			}
			return cs[i].Detail < cs[j].Detail
		})
		fmt.Fprintf(w, "[%s]\n", cat)
		for _, c := range cs {
			line := fmt.Sprintf("  %s %s", c.Kind, c.ID)
			if c.Detail != "" {
				line += ": " + c.Detail
			}
			fmt.Fprintln(w, line)
		}
	}
}

// RenderJSON prints a JSON array of change records.
func RenderJSON(w io.Writer, changes []Change) error {
	if changes == nil {
		changes = []Change{}
	}
	buf, err := json.MarshalIndent(changes, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(buf, '\n'))
	return err
}

// HasBreaking reports whether any change has category BREAKING.
func HasBreaking(changes []Change) bool {
	for _, c := range changes {
		if c.Category == CategoryBreaking {
			return true
		}
	}
	return false
}

// Summary returns a one-line human summary.
func Summary(changes []Change) string {
	if len(changes) == 0 {
		return "no changes"
	}
	counts := map[string]int{}
	for _, c := range changes {
		counts[c.Category]++
	}
	var parts []string
	for _, cat := range []string{CategoryBreaking, CategoryRemoved, CategoryAdded, CategoryChanged} {
		if n := counts[cat]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, strings.ToLower(cat)))
		}
	}
	return strings.Join(parts, ", ")
}

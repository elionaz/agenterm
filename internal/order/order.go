// Package order rearranges id lists for the reorder methods (projects,
// sessions of a project).
package order

import "fmt"

// Front returns all with ids moved to the front, in that order; the other
// ids keep their relative order after them. Every id must be in all, once.
func Front(all, ids []string) ([]string, error) {
	in := make(map[string]bool, len(all))
	for _, id := range all {
		in[id] = true
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(all))
	for _, id := range ids {
		if !in[id] {
			return nil, fmt.Errorf("not found: %s", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("%s listed twice", id)
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, id := range all {
		if !seen[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// Within reorders only the members of a group inside all: the positions
// they occupy are refilled with Front(group, ids), and every other id stays
// where it is. ids must all be members.
func Within(all []string, member func(string) bool, ids []string) ([]string, error) {
	var group []string
	for _, id := range all {
		if member(id) {
			group = append(group, id)
		}
	}
	for _, id := range ids {
		if !member(id) {
			return nil, fmt.Errorf("not found: %s", id)
		}
	}
	next, err := Front(group, ids)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(all))
	k := 0
	for i, id := range all {
		if member(id) {
			out[i] = next[k]
			k++
		} else {
			out[i] = id
		}
	}
	return out, nil
}

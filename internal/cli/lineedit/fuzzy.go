package lineedit

// fuzzyMatch reports whether pattern is a (case-insensitive) subsequence of s,
// with a relevance score and the matched rune indices in s. Higher is better:
// consecutive matches and matches at a word/segment boundary (start of string,
// or after a non-word rune like '/') score far above scattered ones, and an
// earlier first match and a tighter overall span are preferred. This makes
// "/mdl" still find "/models" and survives a typo'd middle, while exact prefixes
// stay on top. ok=false when some pattern rune can't be matched in order.
func fuzzyMatch(pattern, s string) (score int, positions []int, ok bool) {
	pr := []rune(pattern)
	sr := []rune(s)
	if len(pr) == 0 {
		return 0, nil, true
	}
	positions = make([]int, 0, len(pr))
	pi := 0
	prev := -2
	for i := 0; i < len(sr) && pi < len(pr); i++ {
		if lowerRune(sr[i]) != lowerRune(pr[pi]) {
			continue
		}
		cell := 1
		if i == prev+1 {
			cell += 6 // consecutive run
		}
		if i == 0 || !isWordRune(sr[i-1]) {
			cell += 4 // word / segment boundary (e.g. the char after '/')
		}
		score += cell
		positions = append(positions, i)
		prev = i
		pi++
	}
	if pi < len(pr) {
		return 0, nil, false
	}
	// Prefer an earlier first match and a shorter candidate, mildly.
	score -= positions[0]
	score -= (len(sr) - len(pr)) / 4
	return score, positions, true
}

func lowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

func isWordRune(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}

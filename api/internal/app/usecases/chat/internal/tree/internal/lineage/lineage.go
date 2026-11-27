package lineage

func Walk(
	id string,
	parentOf func(string) string,
	isChat func(string) bool,
) []string {
	var out []string
	seen := map[string]bool{id: true}
	for at := parentOf(id); at != "" && !seen[at]; at = parentOf(at) {
		seen[at] = true
		if isChat(at) {
			out = append(out, at)
		}
	}
	return out
}

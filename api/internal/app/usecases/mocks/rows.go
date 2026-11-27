package mocks

func upsert[T any](rows []T, item T, id func(T) string) []T {
	for i := range rows {
		if id(rows[i]) == id(item) {
			rows[i] = item
			return rows
		}
	}
	return append(rows, item)
}

func without[T any](rows []T, gone string, id func(T) string) []T {
	kept := rows[:0]
	for _, r := range rows {
		if id(r) != gone {
			kept = append(kept, r)
		}
	}
	return kept
}

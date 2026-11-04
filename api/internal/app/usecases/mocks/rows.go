package mocks

// upsert replaces the row with item's id, or appends item: Save on the real store.
func upsert[T any](rows []T, item T, id func(T) string) []T {
	for i := range rows {
		if id(rows[i]) == id(item) {
			rows[i] = item
			return rows
		}
	}
	return append(rows, item)
}

// without drops gone's row, compacting in place, so the fake holds the net persisted state.
func without[T any](rows []T, gone string, id func(T) string) []T {
	kept := rows[:0]
	for _, r := range rows {
		if id(r) != gone {
			kept = append(kept, r)
		}
	}
	return kept
}

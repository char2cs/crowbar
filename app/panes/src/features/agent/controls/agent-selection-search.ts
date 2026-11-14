// Cheap subsequence fuzzy score: every query char must appear in order;
// contiguous runs score higher so "hgh" ranks "high" over "highlight".
export function fuzzyScore(query: string, text: string): number {
  if (!query) return 1
  const q = query.toLowerCase()
  const t = text.toLowerCase()
  let qi = 0
  let score = 0
  let streak = 0
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) {
      qi++
      streak++
      score += streak
    } else {
      streak = 0
    }
  }
  return qi === q.length ? score + 1 : 0
}

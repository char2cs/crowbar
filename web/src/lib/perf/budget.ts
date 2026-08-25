/**
 * Regression budgets over recorded perf spans (spec: "budgets that fail when
 * a number regresses, not as a report someone reads"). Pure functions —
 * gathering the `measures` this reads is a live-app concern, not this
 * module's.
 */
export interface PerfMeasure {
  name: string
  duration: number
}

export interface BudgetViolation {
  name: string
  observedMs: number
  budgetMs: number
  overBy: number
}

interface Summary {
  count: number
  maxMs: number
  p95Ms: number
}

function percentile(sorted: number[], p: number): number {
  if (sorted.length === 0) return 0
  const index = Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1)
  return sorted[Math.max(0, index)]
}

/** One summary row per distinct measure name: how many samples, the worst
 *  one, and the 95th percentile — a single GC-pause outlier should not by
 *  itself decide a budget verdict, but a sustained slowdown must. */
export function summarize(measures: PerfMeasure[]): Map<string, Summary> {
  const byName = new Map<string, number[]>()
  for (const m of measures) {
    const list = byName.get(m.name)
    if (list) list.push(m.duration)
    else byName.set(m.name, [m.duration])
  }
  const out = new Map<string, Summary>()
  for (const [name, durations] of byName) {
    const sorted = [...durations].sort((a, b) => a - b)
    out.set(name, {
      count: sorted.length,
      maxMs: sorted[sorted.length - 1],
      p95Ms: percentile(sorted, 95),
    })
  }
  return out
}

/** Flags every measure name whose p95 exceeds `budgets[name] * (1 +
 *  toleranceRatio)`. A name absent from `budgets` is not checked — Phase 0/1
 *  only tracks the three spans already instrumented; a later phase adds more
 *  budget entries as it instruments more spans. */
export function checkBudgets(
  measures: PerfMeasure[],
  budgets: Record<string, number>,
  toleranceRatio = 0.15,
): BudgetViolation[] {
  const summary = summarize(measures)
  const violations: BudgetViolation[] = []
  for (const [name, budgetMs] of Object.entries(budgets)) {
    const row = summary.get(name)
    if (!row) continue
    const ceiling = budgetMs * (1 + toleranceRatio)
    if (row.p95Ms > ceiling) {
      violations.push({ name, observedMs: row.p95Ms, budgetMs, overBy: row.p95Ms - budgetMs })
    }
  }
  return violations
}

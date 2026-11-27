export interface Selection {
  providerId: string
  model: string
  effort: string
}

export interface StagedSelection {
  pick: Selection
  base: Selection
}

export function stageSelection(base: Selection, pick: Selection): StagedSelection {
  return { pick, base }
}

const same = (a: Selection, b: Selection) =>
  a.providerId === b.providerId && a.model === b.model && a.effort === b.effort

export function effectiveSelection(staged: StagedSelection | null, live: Selection): Selection {
  return staged && same(staged.base, live) ? staged.pick : live
}

export function declaredModel(provider: { models?: string[] } | undefined, model: string): string {
  const known = provider?.models ?? []
  return known.length === 0 || known.includes(model) ? model : ''
}

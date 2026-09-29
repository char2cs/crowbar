export interface Selection {
  providerId: string
  model: string
  effort: string
}

/** A pick not yet sent, remembered with the chat's selection when it was made. */
export interface StagedSelection {
  pick: Selection
  base: Selection
}

export function stageSelection(base: Selection, pick: Selection): StagedSelection {
  return { pick, base }
}

const same = (a: Selection, b: Selection) =>
  a.providerId === b.providerId && a.model === b.model && a.effort === b.effort

/** The pick applies only while the chat is as it was; any other change wins. */
export function effectiveSelection(staged: StagedSelection | null, live: Selection): Selection {
  return staged && same(staged.base, live) ? staged.pick : live
}

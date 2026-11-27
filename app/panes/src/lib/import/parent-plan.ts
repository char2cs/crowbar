export interface PRLink {
  head: string
  base: string
}

export interface BranchEntry {
  name: string
  isProtected: boolean
  hasWorkspace: boolean
}

export interface ImportPlan {
  importCount: number
  parentCount: number
}

export function computeImportPlan(
  selected: string[],
  prLinks: PRLink[],
  branches: BranchEntry[],
  defaultBranch: string,
): ImportPlan {
  const base = new Map<string, string>()
  for (const l of prLinks) if (l.head && l.base) base.set(l.head, l.base)

  const imported = new Set<string>()
  const protectedSet = new Set<string>()
  for (const b of branches) {
    if (b.hasWorkspace) imported.add(b.name)
    if (b.isProtected) protectedSet.add(b.name)
  }
  const selectedSet = new Set(selected)

  const parents = new Set<string>()
  for (const branch of selected) {
    const visited = new Set<string>([branch])
    let cur = base.get(branch)
    while (cur && cur !== defaultBranch && !visited.has(cur)) {
      visited.add(cur)
      if (imported.has(cur) || protectedSet.has(cur)) break
      if (!selectedSet.has(cur)) parents.add(cur)
      cur = base.get(cur)
    }
  }
  return { importCount: selected.length, parentCount: parents.size }
}

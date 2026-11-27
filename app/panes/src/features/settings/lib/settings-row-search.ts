import type { ReactNode } from 'react'

export function settingRowMatchesQuery(
  query: string,
  label: string,
  description?: ReactNode,
): boolean {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  if (label.toLowerCase().includes(normalized)) return true
  return typeof description === 'string' && description.toLowerCase().includes(normalized)
}

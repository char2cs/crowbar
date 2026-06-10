// Stub
export const SIDEBAR_ITEM_ORDER: string[] = ['file-explorer', 'search', 'extensions']
export const SIDEBAR_ACTIVITY_ITEM_IDS: string[] = SIDEBAR_ITEM_ORDER

export const BOTTOM_PANE_ITEM_ORDER: string[] = ['terminal']

export const FOOTER_LEADING_ITEM_IDS: string[] = ['branch', 'sync']

export const FOOTER_TRAILING_ITEM_IDS: string[] = ['notifications', 'settings']

export const HEADER_TRAILING_ITEM_IDS: string[] = ['layout', 'extensions']

export type FooterLeadingItemId = (typeof FOOTER_LEADING_ITEM_IDS)[number]
export type FooterTrailingItemId = (typeof FOOTER_TRAILING_ITEM_IDS)[number]
export type HeaderTrailingItemId = (typeof HEADER_TRAILING_ITEM_IDS)[number]
export type SidebarActivityItemId = (typeof SIDEBAR_ACTIVITY_ITEM_IDS)[number]

export function normalizeItemOrder<T extends string>(ids: T[], order: string[]): T[] {
  const ordered = order.filter((id) => ids.includes(id as T)) as T[]
  const remaining = ids.filter((id) => !ordered.includes(id))
  return [...ordered, ...remaining]
}

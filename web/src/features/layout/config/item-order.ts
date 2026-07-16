// Stub
export const SIDEBAR_ITEM_ORDER: string[] = ['file-explorer', 'search', 'extensions']
export const SIDEBAR_ACTIVITY_ITEM_IDS: string[] = SIDEBAR_ITEM_ORDER

export const FOOTER_LEADING_ITEM_IDS: string[] = ['branch', 'sync']

export const FOOTER_TRAILING_ITEM_IDS: string[] = ['settings']

export const HEADER_TRAILING_ITEM_IDS: string[] = ['layout', 'extensions']

export type FooterLeadingItemId = (typeof FOOTER_LEADING_ITEM_IDS)[number]
export type FooterTrailingItemId = (typeof FOOTER_TRAILING_ITEM_IDS)[number]
export type HeaderTrailingItemId = (typeof HEADER_TRAILING_ITEM_IDS)[number]
export type SidebarActivityItemId = (typeof SIDEBAR_ACTIVITY_ITEM_IDS)[number]

export function normalizeItemOrder<T extends string>(ids: T[], order: string[]): T[] {
  // react-doctor-disable-next-line js-set-map-lookups -- `ids` is one of this file's fixed UI-chrome item lists (footer/header/sidebar activity ids, 2-5 entries each); a Set here would cost readability for no measurable gain.
  const ordered = order.filter((id) => ids.includes(id as T)) as T[]
  // react-doctor-disable-next-line js-set-map-lookups -- `ordered` is bounded by the same tiny fixed item list.
  const remaining = ids.filter((id) => !ordered.includes(id))
  return [...ordered, ...remaining]
}

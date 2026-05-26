// Stub: extension UI views are out of scope for this session.
import type React from "react"

export interface ExtensionViewEntry {
  extensionId: string
  id: string
  title: string
  icon: string
  render: () => React.ReactNode
}

export function useExtensionViews(): Map<string, ExtensionViewEntry> {
  return new Map()
}

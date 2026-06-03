// Stub
export interface ExtensionAction {
  id: string
  render: () => React.ReactNode
}

export function useExtensionActions(): { left: ExtensionAction[]; right: ExtensionAction[] } {
  return { left: [], right: [] }
}

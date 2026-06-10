// Stub
export function useProFeature(_featureId?: string): {
  hasAccess: boolean
  showUpgrade: () => void
  isPro: boolean
  isAuthenticated: boolean
} {
  return { hasAccess: false, showUpgrade: () => {}, isPro: false, isAuthenticated: false }
}

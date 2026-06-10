// Stub: ExtensionErrorBoundary is out of scope for this session.
import type { ReactNode } from 'react'
export interface Props {
  children: ReactNode
  fallback?: ReactNode
  extensionId?: string
  name?: string
}
export function ExtensionErrorBoundary({ children }: Props) {
  return <>{children}</>
}

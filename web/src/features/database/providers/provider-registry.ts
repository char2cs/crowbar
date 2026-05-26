// Stub: database feature is out of scope for this session.
import type { DatabaseType } from '../models/provider.types'
import type React from 'react'

export interface DatabaseViewerProps {
  databasePath?: string
  connectionId?: string
}

export interface ProviderConfig {
  isFileBased: boolean
  viewerComponent: () => Promise<{ default: React.ComponentType<DatabaseViewerProps> }>
}

export const PROVIDER_REGISTRY: Record<DatabaseType, ProviderConfig> = {}

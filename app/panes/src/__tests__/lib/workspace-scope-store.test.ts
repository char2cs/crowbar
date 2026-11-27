import { describe, expect, it } from 'vitest'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'

describe('useWorkspaceScopeStore', () => {
  it('starts with no scopes and no listeners', () => {
    const { scopes, listeners } = useWorkspaceScopeStore.getState()
    expect([scopes, listeners]).toEqual([{}, {}])
  })
})

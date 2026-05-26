import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/components/ui/toast', () => ({
  toast: {
    show: vi.fn(),
    dismissByKey: vi.fn(),
  },
}))

import { useLspStore } from '@/features/editor/lsp/lsp-store'
import { toast } from '@/components/ui/toast'

describe('lspStore', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useLspStore.setState((state) => ({
      ...state,
      lspStatus: {
        ...state.lspStatus,
        lastError: undefined,
        status: 'disconnected' as const,
      },
    }))
  })

  it('setLspError updates lspStatus.lastError without calling toast.show', () => {
    useLspStore.getState().actions.setLspError('language server crashed')

    expect(useLspStore.getState().lspStatus.lastError).toBe('language server crashed')
    expect(useLspStore.getState().lspStatus.status).toBe('error')
    expect(toast.show).not.toHaveBeenCalled()
  })

  it('clearLspError clears lspStatus.lastError without calling toast.dismissByKey', () => {
    useLspStore.setState((state) => ({
      ...state,
      lspStatus: {
        ...state.lspStatus,
        lastError: 'some error',
        status: 'error' as const,
        activeWorkspaces: ['ws1'],
      },
    }))

    useLspStore.getState().actions.clearLspError()

    expect(useLspStore.getState().lspStatus.lastError).toBeUndefined()
    expect(toast.dismissByKey).not.toHaveBeenCalled()
  })
})

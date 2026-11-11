import { PERMISSION_LEVEL_OPTIONS } from '@/features/agent/api/agent-providers-api'
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))
beforeEach(() => {
  resetChatStore()
  writeChat({ id: 'c1', permissionLevel: 'trusted' })
})
afterEach(() => {
  platform.mobile = false
})

const permissionLevel = {
  options: PERMISSION_LEVEL_OPTIONS.filter((opt) => opt.value !== 'full-auto'),
  level: 'trusted' as const,
  setLevel: vi.fn(),
  cycle: vi.fn(() => true),
}
const permission = { wsId: 'w1', chatId: 'c1', levels: ['guarded' as const, 'trusted' as const] }

// The blank document and the dock's footer both draw this cluster, so both name the permission level.
describe('SelectionCluster', () => {
  it('carries the permission chip on the desktop, after the model', () => {
    render(
      <SelectionCluster
        providers={[]}
        model=""
        effort=""
        permission={permission}
        permissionLevel={permissionLevel}
        onSelectionChange={vi.fn()}
      />,
    )
    expect(screen.getByTestId('chat-permission-level-chip')).toHaveTextContent('Trusted')
  })

  it('leaves the level to the model sheet on the phone', () => {
    platform.mobile = true
    render(
      <SelectionCluster
        providers={[]}
        model=""
        effort=""
        permission={permission}
        permissionLevel={permissionLevel}
        onSelectionChange={vi.fn()}
      />,
    )
    expect(screen.queryByTestId('chat-permission-level-chip')).not.toBeInTheDocument()
  })
})

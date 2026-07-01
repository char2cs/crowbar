import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'

vi.mock('@/features/window/stores/toast-store', () => ({ toast: { show: vi.fn() } }))

import { toast } from '@/features/window/stores/toast-store'
import { useSidebarStore } from '@/lib/store/sidebar'
import { PlaceholderToastWatcher } from '@/components/layout/placeholder-toast-watcher'
import type { Repo } from '@/lib/store/sidebar'

const repoWith = (over = {}): Repo => ({
  id: 'r1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: 'bg-sky-700',
  workspaces: [{ id: 'ph', branch: 'develop', status: 'locked', heldByPath: '/repo', age: '' }],
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState({ repos: [] })
})

describe('PlaceholderToastWatcher', () => {
  it('fires an error toast with a Fix action once per new placeholder', () => {
    useSidebarStore.setState({ repos: [repoWith()] })
    render(<PlaceholderToastWatcher />)
    expect(toast.show).toHaveBeenCalledTimes(1)
    const arg = (toast.show as unknown as { mock: { calls: unknown[][] } }).mock.calls[0][0] as {
      type: string
      key: string
      action: { label: string }
    }
    expect(arg.type).toBe('error')
    expect(arg.key).toBe('ph')
    expect(arg.action.label).toMatch(/fix/i)
  })

  it('does not re-fire for an already-seen placeholder on re-render', () => {
    useSidebarStore.setState({ repos: [repoWith()] })
    const { rerender } = render(<PlaceholderToastWatcher />)
    rerender(<PlaceholderToastWatcher />)
    expect(toast.show).toHaveBeenCalledTimes(1)
  })
})

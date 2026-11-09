import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useTerminalSearch } from '@/features/terminal/hooks/use-terminal-search'
import type { TerminalAddons } from '@/features/terminal/hooks/use-terminal-addons'

const OPTIONS = { caseSensitive: false, wholeWord: false, regex: false }

afterEach(() => {
  document.documentElement.style.removeProperty('--primary')
})

describe('useTerminalSearch', () => {
  it('fills matches with the theme primary, not a fixed blue', () => {
    document.documentElement.style.setProperty('--primary', '#606c38')
    const findNext = vi.fn(() => true)
    const addons = {
      searchAddon: { findNext, onDidChangeResults: () => ({ dispose: () => {} }) },
    } as unknown as TerminalAddons
    const { result } = renderHook(() => useTerminalSearch(null, addons))

    act(() => result.current.barProps.onSearch('needle', OPTIONS))

    const call = findNext.mock.calls[0] as unknown as [
      string,
      { decorations: { matchBackground: string } },
    ]
    expect(call[1].decorations.matchBackground).toBe('#606c38')
  })
})

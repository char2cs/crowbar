import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest'

describe('editor settings sync debounce', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not call syncEditorSettings more than once for rapid settings changes', async () => {
    // Import AFTER setting up fake timers so the module-level code sees them
    const { useSettingsStore } = await import('@/features/settings/store')
    const { useEditorSettingsStore } = await import('@/features/editor/stores/settings-store')

    const setFontSize = vi.spyOn(
      useEditorSettingsStore.getState().actions,
      'setFontSize'
    )

    // Trigger 5 rapid settings changes
    for (let i = 0; i < 5; i++) {
      useSettingsStore.setState((s) => ({
        settings: { ...s.settings, fontSize: 12 + i },
      }))
    }

    // Before the debounce fires, no sync should have happened from subscriptions
    // (the initial sync at module load is separate)
    const callsBefore = setFontSize.mock.calls.length

    // Advance timers past the debounce window
    vi.advanceTimersByTime(100)

    // Only ONE additional call should have fired (the debounced one)
    expect(setFontSize.mock.calls.length).toBe(callsBefore + 1)
  })
})

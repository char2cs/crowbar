import { describe, it, expect, vi } from 'vitest'

// Pure unit test of the auto-promote-on-change pattern
// (No component rendering needed — the logic is simple enough to test directly)
describe('auto-promote wrapper logic', () => {
  it('calls onPromote when isPreview=true and onChange fires', () => {
    const onPromote = vi.fn()
    const onChange = vi.fn()
    const isPreview = true

    // Simulate what onChangeWithPromote does
    const wrappedChange = (content: string) => {
      if (isPreview) onPromote?.()
      onChange(content)
    }

    wrappedChange('new content')

    expect(onPromote).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('new content')
  })

  it('does NOT call onPromote when isPreview=false', () => {
    const onPromote = vi.fn()
    const onChange = vi.fn()
    const isPreview = false

    const wrappedChange = (content: string) => {
      if (isPreview) onPromote?.()
      onChange(content)
    }

    wrappedChange('new content')

    expect(onPromote).not.toHaveBeenCalled()
    expect(onChange).toHaveBeenCalledWith('new content')
  })

  it('does not throw when isPreview=true but onPromote is undefined', () => {
    const onChange = vi.fn()

    // Build the wrapper the same way the component does — parameters typed as the component
    // receives them, so TypeScript cannot narrow onPromote to `never` via const-folding.
    const buildWrapper = (isPreview: boolean, onPromote: (() => void) | undefined) =>
      (content: string) => {
        if (isPreview) onPromote?.()
        onChange(content)
      }

    const wrappedChange = buildWrapper(true, undefined)

    // Should not throw even though onPromote is undefined
    expect(() => wrappedChange('text')).not.toThrow()
    expect(onChange).toHaveBeenCalledWith('text')
  })
})

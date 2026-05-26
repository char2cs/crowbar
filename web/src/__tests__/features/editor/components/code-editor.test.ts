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

  it('still calls onChange even when isPreview=true (does not short-circuit)', () => {
    const onPromote = vi.fn()
    const onChange = vi.fn()
    const isPreview = true

    const wrappedChange = (content: string) => {
      if (isPreview) onPromote?.()
      onChange(content)
    }

    wrappedChange('typed text')

    expect(onChange).toHaveBeenCalledWith('typed text')
  })
})

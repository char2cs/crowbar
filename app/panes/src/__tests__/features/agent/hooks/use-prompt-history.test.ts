import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { usePromptHistory } from '@/features/agent/hooks/use-prompt-history'

describe('usePromptHistory', () => {
  it('walks older turns newest-first, then newer turns back to the live draft', () => {
    const { result } = renderHook(() => usePromptHistory(() => ['first', 'second', 'third']))

    expect(result.current.recallOlder('typing…')).toBe('third')
    expect(result.current.recallOlder('')).toBe('second')
    expect(result.current.recallOlder('')).toBe('first')
    expect(result.current.recallOlder('')).toBeUndefined()

    expect(result.current.recallNewer()).toBe('second')
    expect(result.current.recallNewer()).toBe('third')
    expect(result.current.recallNewer()).toBe('typing…')
    expect(result.current.recallNewer()).toBeUndefined()
  })

  it('has nothing to recall with no turns loaded yet', () => {
    const { result } = renderHook(() => usePromptHistory(() => []))
    expect(result.current.recallOlder('draft')).toBeUndefined()
    expect(result.current.recallNewer()).toBeUndefined()
  })

  it('reset() abandons the walk — the next recallOlder starts over and re-stashes', () => {
    const { result } = renderHook(() => usePromptHistory(() => ['only one']))

    expect(result.current.recallOlder('typing…')).toBe('only one')
    result.current.reset()

    expect(result.current.recallOlder('edited since')).toBe('only one')
    expect(result.current.recallNewer()).toBe('edited since')
  })

  it('searches back through earlier prompts for the words typed, as the terminal does on ctrl+r', () => {
    const { result } = renderHook(() =>
      usePromptHistory(() => ['run the tests', 'fix lint', 'run the tests again', 'deploy']),
    )

    expect(result.current.searchOlder('tests')).toBe('run the tests again')
    expect(result.current.searchOlder('run the tests again')).toBe('run the tests')
    expect(result.current.searchOlder('run the tests')).toBeUndefined()
    expect(result.current.recallNewer()).toBe('fix lint')
  })

  it('finds nothing for words no earlier prompt has', () => {
    const { result } = renderHook(() => usePromptHistory(() => ['one', 'two']))
    expect(result.current.searchOlder('three')).toBeUndefined()
    expect(result.current.recallNewer()).toBeUndefined()
  })
})

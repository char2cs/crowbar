import { describe, it, expect } from 'vitest'
import { idle, loading, success, failed, dataOf, fetchedAtOf } from '@/lib/loadable'

describe('loadable', () => {
  it('idle has no data', () => {
    expect(dataOf(idle())).toBeUndefined()
    expect(fetchedAtOf(idle())).toBeUndefined()
  })

  it('success carries data and fetchedAt', () => {
    const l = success([1, 2], 1000)
    expect(l.status).toBe('success')
    expect(dataOf(l)).toEqual([1, 2])
    expect(fetchedAtOf(l)).toBe(1000)
  })

  it('loading from a previous success carries stale data', () => {
    const l = loading(success(['x'], 500))
    expect(l.status).toBe('loading')
    expect(dataOf(l)).toEqual(['x'])
    expect(fetchedAtOf(l)).toBe(500)
  })

  it('loading from idle has no stale data', () => {
    const l = loading(idle())
    expect(dataOf(l)).toBeUndefined()
  })

  it('failed preserves stale data from previous success', () => {
    const l = failed(new Error('boom'), success(['y'], 700))
    expect(l.status).toBe('error')
    if (l.status === 'error') expect(l.error.message).toBe('boom')
    expect(dataOf(l)).toEqual(['y'])
    expect(fetchedAtOf(l)).toBe(700)
  })

  it('failed from loading-with-stale keeps the stale data', () => {
    const l = failed(new Error('x'), loading(success(['z'], 900)))
    expect(dataOf(l)).toEqual(['z'])
    expect(fetchedAtOf(l)).toBe(900)
  })

  it('failed from idle has null stale data', () => {
    const l = failed(new Error('x'), idle())
    expect(l.status).toBe('error')
    expect(dataOf(l)).toBeUndefined()
  })
})

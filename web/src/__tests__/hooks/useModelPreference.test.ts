import { renderHook, act } from '@testing-library/react'
import { useModelPreference } from '@/hooks/useModelPreference'
import { beforeEach, expect, test } from 'vitest'

beforeEach(() => localStorage.clear())

test('defaults to Sonnet 4.6', () => {
  const { result } = renderHook(() => useModelPreference())
  expect(result.current.model).toBe('claude-sonnet-4-6')
})

test('setModel persists to localStorage', () => {
  const { result } = renderHook(() => useModelPreference())
  act(() => result.current.setModel('claude-haiku-4-5-20251001'))
  expect(localStorage.getItem('crowbar.model')).toBe('claude-haiku-4-5-20251001')
})

test('reads from localStorage on mount', () => {
  localStorage.setItem('crowbar.model', 'claude-opus-4-7')
  const { result } = renderHook(() => useModelPreference())
  expect(result.current.model).toBe('claude-opus-4-7')
})

test('returns all three model options', () => {
  const { result } = renderHook(() => useModelPreference())
  expect(result.current.models).toHaveLength(3)
})

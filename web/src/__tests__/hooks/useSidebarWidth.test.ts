import { renderHook, act } from '@testing-library/react'
import { useSidebarWidth } from '@/hooks/useSidebarWidth'

beforeEach(() => localStorage.clear())

test('defaults to 256', () => {
  const { result } = renderHook(() => useSidebarWidth())
  expect(result.current.width).toBe(256)
})

test('restores persisted value from localStorage', () => {
  localStorage.setItem('crowbar-sidebar-width', '320')
  const { result } = renderHook(() => useSidebarWidth())
  expect(result.current.width).toBe(320)
})

test('clamps stored value to min 180', () => {
  localStorage.setItem('crowbar-sidebar-width', '50')
  const { result } = renderHook(() => useSidebarWidth())
  expect(result.current.width).toBe(180)
})

test('clamps stored value to max 400', () => {
  localStorage.setItem('crowbar-sidebar-width', '999')
  const { result } = renderHook(() => useSidebarWidth())
  expect(result.current.width).toBe(400)
})

test('persists width changes to localStorage', () => {
  const { result } = renderHook(() => useSidebarWidth())
  act(() => result.current.setWidth(300))
  expect(localStorage.getItem('crowbar-sidebar-width')).toBe('300')
})

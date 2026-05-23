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

test('NaN in localStorage falls back to default', () => {
  localStorage.setItem('crowbar-sidebar-width', 'not-a-number')
  const { result } = renderHook(() => useSidebarWidth())
  expect(result.current.width).toBe(256)
})

test('startResize increases width on mouse right drag', () => {
  const { result } = renderHook(() => useSidebarWidth())

  // Simulate mousedown at x=100
  const mousedownEvent = { clientX: 100, preventDefault: () => {} } as unknown as React.MouseEvent
  act(() => { result.current.startResize(mousedownEvent) })

  // Simulate mousemove to x=150 (50px right)
  act(() => {
    document.dispatchEvent(new MouseEvent('mousemove', { clientX: 150 }))
  })

  expect(result.current.width).toBe(256 + 50) // 306

  // Simulate mouseup to clean up
  act(() => {
    document.dispatchEvent(new MouseEvent('mouseup'))
  })
})

test('startResize clamps at max on large rightward drag', () => {
  const { result } = renderHook(() => useSidebarWidth())

  const mousedownEvent = { clientX: 0, preventDefault: () => {} } as unknown as React.MouseEvent
  act(() => { result.current.startResize(mousedownEvent) })

  act(() => {
    document.dispatchEvent(new MouseEvent('mousemove', { clientX: 500 }))
  })

  expect(result.current.width).toBe(400) // clamped to MAX

  act(() => { document.dispatchEvent(new MouseEvent('mouseup')) })
})

import { afterEach, describe, expect, it } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useSurfaceThemeColor } from '@/features/window/hooks/use-surface-theme-color'

const themeColor = () =>
  document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content

afterEach(() => {
  document.body.removeAttribute('style')
  document.querySelector('meta[name="theme-color"]')?.remove()
})

describe('useSurfaceThemeColor', () => {
  it('publishes the body surface as an opaque colour', () => {
    document.body.style.backgroundColor = 'rgba(252, 252, 250, 0.6)'
    renderHook(() => useSurfaceThemeColor())
    expect(themeColor()).toBe('rgb(252, 252, 250)')
  })

  it('leaves an opaque surface alone', () => {
    document.body.style.backgroundColor = 'rgb(10, 10, 9)'
    renderHook(() => useSurfaceThemeColor())
    expect(themeColor()).toBe('rgb(10, 10, 9)')
  })
})

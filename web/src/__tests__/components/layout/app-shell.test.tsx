import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))
vi.mock('@/components/layout/ide-shell', () => ({ IDEShell: () => <p>ide shell</p> }))
vi.mock('@/components/layout/mobile/mobile-shell', () => ({
  MobileShell: () => <p>mobile shell</p>,
}))

const { AppShell } = await import('@/components/layout/app-shell')

afterEach(() => {
  platform.mobile = false
})

describe('AppShell', () => {
  it('lays the desktop app out as the IDE shell', () => {
    render(<AppShell />)
    expect(screen.getByText('ide shell')).toBeInTheDocument()
  })

  it('lays the phone app out as the single-pane mobile shell', async () => {
    platform.mobile = true
    render(<AppShell />)
    expect(await screen.findByText('mobile shell')).toBeInTheDocument()
  })
})

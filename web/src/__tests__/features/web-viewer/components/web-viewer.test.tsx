import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: vi.fn().mockReturnValue(false) }))

import { isTauri } from '@/lib/crowbar-bridge'
import { WebViewer } from '@/features/web-viewer/components/web-viewer'

describe('WebViewer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders an iframe with a crowbar-browser src in Tauri mode', () => {
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(true)

    render(<WebViewer url="https://example.com" bufferId="b1" isVisible />)

    const iframe = document.querySelector('iframe')
    expect(iframe).toBeTruthy()
    expect(iframe?.src).toContain('crowbar-browser://proxy/https/example.com')
  })

  it('renders an iframe in non-Tauri mode (dev fallback)', () => {
    render(<WebViewer url="https://example.com" bufferId="b1" isVisible />)

    const iframe = document.querySelector('iframe')
    expect(iframe).toBeTruthy()
    expect(iframe?.src).toContain('example.com')
  })

  it('shows the URL in the address bar', () => {
    render(<WebViewer url="https://example.com" bufferId="b1" isVisible />)
    const input = screen.getByPlaceholderText('Enter URL or search…')
    expect(input).toHaveValue('https://example.com')
  })

  it('updates address bar when postMessage nav event arrives', async () => {
    render(<WebViewer url="https://example.com" bufferId="b1" isVisible />)

    await act(async () => {
      const iframe = document.querySelector('iframe')
      const event = new MessageEvent('message', {
        data: {
          type: '__crowbar_browser_nav__',
          url: 'https://example.com/about',
          canGoBack: true,
          canGoForward: false,
        },
        // Set source to match the iframe so the handler accepts it
        source: iframe?.contentWindow ?? null,
      })
      window.dispatchEvent(event)
    })

    const input = screen.getByPlaceholderText('Enter URL or search…')
    expect(input).toHaveValue('https://example.com/about')
  })

  it('normalizes bare domain in address bar on submit', async () => {
    const user = userEvent.setup()
    render(<WebViewer url="about:blank" bufferId="b1" isVisible />)

    const input = screen.getByPlaceholderText('Enter URL or search…')
    await user.clear(input)
    await user.type(input, 'github.com')
    await user.keyboard('{Enter}')

    expect(input).toHaveValue('https://github.com')
  })

  it('falls back to Google search for non-URL input', async () => {
    const user = userEvent.setup()
    render(<WebViewer url="about:blank" bufferId="b1" isVisible />)

    const input = screen.getByPlaceholderText('Enter URL or search…')
    await user.clear(input)
    await user.type(input, 'how does react work')
    await user.keyboard('{Enter}')

    expect(input).toHaveValue(
      'https://www.google.com/search?q=how%20does%20react%20work',
    )
  })
})

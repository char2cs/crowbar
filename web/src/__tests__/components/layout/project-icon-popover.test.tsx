/**
 * The project row's icon, and the one thing about it that failed silently.
 *
 * A DTO's icon URL is a bare `/v0/...` path. The desktop webview is served from
 * its own origin — the Vite dev server, or the app bundle in a packaged build —
 * so an unresolved path is a request to the wrong server, and the daemon never
 * sees it. The failure mode is the nasty one: the <img> 404s, RepoAvatarImg
 * catches the error and renders the fallback, and an uploaded icon comes out
 * looking exactly like an icon nobody ever set. Nothing throws and nothing is
 * logged.
 *
 * The repo avatar has always been resolved (build-repo-tree.ts, which is a
 * DTO→domain mapper projects simply do not have). These pin that the project
 * icon is resolved too, and through the same helper.
 */
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  // Stand in for the desktop's `crowbar://` unix-socket scheme, which is what
  // API_BASE resolves to there and '' in a plain browser.
  API_BASE: 'crowbar://localhost',
  assetURL: (path: string) => `crowbar://localhost${path}`,
}))

import { ProjectIconPopover } from '@/components/layout/project-icon-popover'

const ICON_PATH = '/v0/projects/p1/icon?v=3'

describe('the project icon', () => {
  it('resolves the DTO path against the API base before handing it to an <img>', () => {
    render(<ProjectIconPopover project={{ id: 'p1', name: 'harbour', avatarUrl: ICON_PATH }} />)

    const img = screen.getByRole('img', { name: 'harbour' }) as HTMLImageElement
    expect(img.getAttribute('src')).toContain('crowbar://localhost/v0/projects/p1/icon')
  })

  it('never hands an <img> a bare daemon path', () => {
    // The regression itself: a src starting at '/' is a request to whatever
    // origin the webview happens to be served from.
    render(<ProjectIconPopover project={{ id: 'p1', name: 'harbour', avatarUrl: ICON_PATH }} />)

    const img = screen.getByRole('img', { name: 'harbour' })
    expect(img.getAttribute('src')!.startsWith('/')).toBe(false)
  })

  it('keeps the DTO’s own cache-buster and adds its own', () => {
    // The URL is stable across uploads — the bytes change behind it — so both
    // the daemon's ?v= and the popover's local bump have to survive.
    render(<ProjectIconPopover project={{ id: 'p1', name: 'harbour', avatarUrl: ICON_PATH }} />)

    expect(screen.getByRole('img', { name: 'harbour' }).getAttribute('src')).toContain('v=3')
  })

  it('draws the Library default when no icon is set, and no <img> at all', () => {
    const { container } = render(<ProjectIconPopover project={{ id: 'p1', name: 'harbour' }} />)

    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('.lucide-library')).not.toBeNull()
  })

  it('renders an emoji directly, with no request for it', () => {
    const { container } = render(
      <ProjectIconPopover project={{ id: 'p1', name: 'harbour', avatarEmoji: '🛰️' }} />,
    )

    expect(container.querySelector('img')).toBeNull()
    expect(container.textContent).toContain('🛰️')
  })

  it('prefers the emoji over a stored image, as the daemon does', () => {
    const { container } = render(
      <ProjectIconPopover
        project={{ id: 'p1', name: 'harbour', avatarUrl: ICON_PATH, avatarEmoji: '🛰️' }}
      />,
    )

    expect(container.querySelector('img')).toBeNull()
    expect(container.textContent).toContain('🛰️')
  })

  it('yields the whole mark to the spinner during an agent turn', () => {
    const { container } = render(
      <ProjectIconPopover project={{ id: 'p1', name: 'harbour', avatarUrl: ICON_PATH }} working />,
    )

    expect(container.querySelector('img')).toBeNull()
    expect(screen.queryByLabelText('Edit harbour icon')).toBeNull()
  })
})

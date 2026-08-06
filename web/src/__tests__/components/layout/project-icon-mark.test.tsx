import { render } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { ProjectIconMark } from '@/components/layout/project-icon-mark'
import { assetURL } from '@/lib/api'

/**
 * The mark a project shows, wherever it is shown. It exists because the context
 * pill used to draw its own <Library>, so a project with an icon rendered that
 * icon in the sidebar row and the default glyph in the pill above it.
 */
describe('ProjectIconMark', () => {
  const project = { name: 'Crowbar' }

  it('draws the Library default when the project has no icon', () => {
    const { container } = render(<ProjectIconMark project={project} size="lg" />)

    expect(container.querySelector('.lucide-library')).not.toBeNull()
    expect(container.querySelector('img')).toBeNull()
  })

  it('prefers an emoji over an uploaded image', () => {
    // The daemon clears a stored image when an emoji is set and vice versa, so
    // the two can never both be live; this order mirrors that, it does not
    // invent one.
    const { container } = render(
      <ProjectIconMark
        project={{ ...project, avatarEmoji: '🚀', avatarUrl: '/v0/projects/p1/icon' }}
        size="lg"
      />,
    )

    expect(container.textContent).toBe('🚀')
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('.lucide-library')).toBeNull()
  })

  it('resolves the image URL for the browser', () => {
    const { container } = render(
      <ProjectIconMark project={{ ...project, avatarUrl: '/v0/projects/p1/icon' }} size="lg" />,
    )

    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      assetURL('/v0/projects/p1/icon'),
    )
  })

  it('scales the mark by size, so one component serves row and pill', () => {
    const box = (size: 'md' | 'lg') => {
      const { container } = render(<ProjectIconMark project={project} size={size} />)
      return container.firstElementChild!.className
    }

    expect(box('lg')).toContain('size-5')
    expect(box('md')).toContain('size-4')
  })

  describe('cache-busting', () => {
    const url = '/v0/projects/p1/icon?v=3'

    it('leaves the daemon-versioned URL alone for read-only surfaces', () => {
      const { container } = render(
        <ProjectIconMark project={{ ...project, avatarUrl: url }} size="md" />,
      )

      expect(container.querySelector('img')?.getAttribute('src')).toBe(assetURL(url))
    })

    it('stacks its own version for a surface that can change the icon', () => {
      // The daemon's `?v=` only moves once the updated ProjectDTO has come back
      // over the WS stream; this one moves as soon as the upload resolves, which
      // is what stops a re-upload showing the previous bytes.
      const { container } = render(
        <ProjectIconMark project={{ ...project, avatarUrl: url }} size="lg" version={7} />,
      )

      expect(container.querySelector('img')?.getAttribute('src')).toBe(`${assetURL(url)}&v=7`)
    })
  })
})

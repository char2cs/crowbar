import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { RepoIconMark, type RepoIconSource } from '@/components/layout/repo-icon-mark'

/**
 * A repo's mark is ONE component across the repo home row, the context pill, the
 * workspace switcher, the removal tray and the New Tab heading.
 *
 * It used to be two. The row drew it through IconPopover (separate emoji/iconUrl
 * props, rounded-md, px-1, a full type step larger); everywhere else drew it
 * through RepoAvatar, which encodes the emoji INSIDE the url as an `emoji:`
 * sentinel and used rounded-sm, px-0.5 and a smaller emoji. Same repo, two
 * different marks — visibly so in the pill sitting directly above the row.
 */
describe('RepoIconMark', () => {
  const repo: RepoIconSource = {
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
  }

  it('draws the generated letter tile when the repo has no icon', () => {
    const { container } = render(<RepoIconMark repo={repo} size="lg" />)
    const tile = container.firstElementChild!
    expect(tile.textContent).toBe('C')
    expect(tile.className).toContain('bg-indigo-700')
    expect(container.querySelector('img')).toBeNull()
  })

  it('renders the emoji the avatarURL sentinel carries', () => {
    const { container } = render(
      <RepoIconMark repo={{ ...repo, avatarURL: 'emoji:🚀' }} size="lg" />,
    )
    expect(container.textContent).toBe('🚀')
    expect(container.querySelector('img')).toBeNull()
  })

  it('renders a real avatarURL as an image', () => {
    const { container } = render(
      <RepoIconMark repo={{ ...repo, avatarURL: '/v0/x/icon?v=2' }} size="lg" />,
    )
    expect(container.querySelector('img')?.getAttribute('src')).toBe('/v0/x/icon?v=2')
  })

  // The repo home row is where an icon is SET, so its rendering is the one every
  // other surface has to match.
  it('matches the repo home row: rounded-md, px-1, and a full step larger emoji', () => {
    const tile = render(<RepoIconMark repo={repo} size="lg" />).container.firstElementChild!
    expect(tile.className).toContain('rounded-md')
    expect(tile.className).toContain('px-1')
    expect(tile.className).not.toContain('rounded-sm')

    const emoji = render(<RepoIconMark repo={{ ...repo, avatarURL: 'emoji:🚀' }} size="lg" />)
      .container.firstElementChild!
    expect(emoji.className).toContain('text-lg')

    const img = render(
      <RepoIconMark repo={{ ...repo, avatarURL: '/i' }} size="lg" />,
    ).container.querySelector('img')!
    expect(img.className).toContain('rounded-md')
  })

  it('scales by size while keeping the same shape', () => {
    const box = (size: 'sm' | 'lg' | 'xl') =>
      render(<RepoIconMark repo={repo} size={size} />).container.firstElementChild!.className
    expect(box('sm')).toContain('size-4')
    expect(box('lg')).toContain('size-5')
    expect(box('xl')).toContain('size-6')
  })

  it('only the editing surface adds a cache-bust', () => {
    const plain = render(
      <RepoIconMark repo={{ ...repo, avatarURL: '/i?v=3' }} size="lg" />,
    ).container.querySelector('img')!
    expect(plain.getAttribute('src')).toBe('/i?v=3')

    const versioned = render(
      <RepoIconMark repo={{ ...repo, avatarURL: '/i?v=3' }} size="lg" version={7} />,
    ).container.querySelector('img')!
    expect(versioned.getAttribute('src')).toBe('/i?v=3&v=7')
  })
})

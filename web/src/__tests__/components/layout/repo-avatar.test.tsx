import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { RepoAvatar, RepoAvatarImg } from '@/components/layout/repo-avatar'

describe('RepoAvatar', () => {
  it('renders the emoji variant for emoji: urls', () => {
    render(
      <RepoAvatar avatar={{ url: 'emoji:🦊', label: 'R', color: 'avatar-rose' }} name="repo" />,
    )
    expect(screen.getByText('🦊')).toBeInTheDocument()
  })

  it('renders an img for icon urls', () => {
    render(
      <RepoAvatar
        avatar={{ url: '/v0/projects/p/repos/r/icon?v=1', label: 'R', color: 'avatar-rose' }}
        name="repo"
      />,
    )
    expect(screen.getByRole('img', { name: 'repo' })).toBeInTheDocument()
  })

  it('renders the letter fallback without a url', () => {
    render(<RepoAvatar avatar={{ label: 'R', color: 'avatar-rose' }} name="repo" />)
    expect(screen.getByText('R')).toBeInTheDocument()
  })

  it('falls back to the letter variant when the image fails to load', () => {
    render(
      <RepoAvatar
        avatar={{ url: '/v0/projects/p/repos/r/icon?v=1', label: 'R', color: 'avatar-rose' }}
        name="repo"
      />,
    )
    fireEvent.error(screen.getByRole('img', { name: 'repo' }))
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.getByText('R')).toBeInTheDocument()
  })
})

describe('RepoAvatarImg', () => {
  it('recovers from an error when the src changes (new ?v= version)', () => {
    const fallback = <span>FB</span>
    const { rerender } = render(<RepoAvatarImg src="/icon?v=1" alt="repo" fallback={fallback} />)
    fireEvent.error(screen.getByRole('img'))
    expect(screen.getByText('FB')).toBeInTheDocument()

    // A new version means new bytes may exist — retry the image.
    rerender(<RepoAvatarImg src="/icon?v=2" alt="repo" fallback={fallback} />)
    expect(screen.getByRole('img')).toBeInTheDocument()
    expect(screen.queryByText('FB')).not.toBeInTheDocument()
  })
})

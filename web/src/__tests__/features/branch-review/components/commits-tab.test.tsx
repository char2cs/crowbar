import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { success, failed, idle } from '@/lib/loadable'
import { useGitStore, type GitData } from '@/features/git/stores/git-store'
import { CommitsTab } from '@/features/branch-review/components/commits-tab'

const gitData = (commits: { hash: string; message: string; date: string }[]) =>
  ({ status: { branch: 'main', files: [], ahead: 0, behind: 0 }, commits, branches: [], stashes: [] }) as unknown as GitData

beforeEach(() => { useGitStore.setState({ gitData: idle() }) })

describe('CommitsTab', () => {
  it('renders commits on success', () => {
    useGitStore.setState({ gitData: success(gitData([{ hash: 'abcdef0', message: 'hello', date: '2026-01-01' }])) })
    render(<CommitsTab repoPath="/repo" />)
    expect(screen.getByText('hello')).toBeInTheDocument()
  })

  it('shows inline error when no cache and fetch failed', () => {
    useGitStore.setState({ gitData: failed(new Error('500'), idle()) })
    render(<CommitsTab repoPath="/repo" />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
  })
})

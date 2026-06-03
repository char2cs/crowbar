import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { failed, success, idle } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import { ProjectListPage } from '@/components/projects/ProjectListPage'

beforeEach(() => { useProjectDataStore.setState({ data: success([]) }) })

describe('ProjectListPage error vs empty', () => {
  it('shows inline error (not onboarding) when the fetch failed with no cache', () => {
    useProjectDataStore.setState({ data: failed(new Error('500'), idle()) })
    render(<ProjectListPage onSelect={() => {}} />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
    expect(screen.queryByText(/no projects yet/i)).toBeNull()
  })

  it('shows onboarding when the fetch succeeded but list is empty', () => {
    useProjectDataStore.setState({ data: success([]) })
    render(<ProjectListPage onSelect={() => {}} />)
    expect(screen.getByText(/no projects yet/i)).toBeInTheDocument()
  })
})

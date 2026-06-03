import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { DataState } from '@/components/ui/data-state'
import { idle, loading, success, failed } from '@/lib/loadable'

const renderList = (data: string[]) => <ul>{data.map(d => <li key={d}>{d}</li>)}</ul>

describe('DataState', () => {
  it('idle renders nothing', () => {
    const { container } = render(<DataState loadable={idle()} onRetry={() => {}}>{renderList}</DataState>)
    expect(container).toBeEmptyDOMElement()
  })

  it('loading with no stale shows a spinner', () => {
    render(<DataState loadable={loading(idle())} onRetry={() => {}} loadingLabel="Loading items">{renderList}</DataState>)
    expect(screen.getByLabelText('Loading items')).toBeInTheDocument()
  })

  it('error with no stale shows inline error', () => {
    render(<DataState loadable={failed(new Error('x'), idle())} onRetry={() => {}}>{renderList}</DataState>)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
  })

  it('success renders children with data', () => {
    render(<DataState loadable={success(['a', 'b'])} onRetry={() => {}}>{renderList}</DataState>)
    expect(screen.getByText('a')).toBeInTheDocument()
    expect(screen.getByText('b')).toBeInTheDocument()
  })

  it('error with stale shows banner and renders stale content', () => {
    const l = failed(new Error('x'), success(['cached'], 1000))
    render(<DataState loadable={l} onRetry={() => {}}>{renderList}</DataState>)
    expect(screen.getByText(/cached data/i)).toBeInTheDocument()
    expect(screen.getByText('cached')).toBeInTheDocument()
  })

  it('success with empty data and emptyMessage shows the empty message', () => {
    render(<DataState loadable={success<string[]>([])} onRetry={() => {}} emptyMessage="Nothing here">{renderList}</DataState>)
    expect(screen.getByText('Nothing here')).toBeInTheDocument()
  })
})

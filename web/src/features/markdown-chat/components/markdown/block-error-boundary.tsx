import { Component, type ReactNode } from 'react'

interface Props {
  children: ReactNode
  label?: string
}

interface State {
  error: Error | null
}

// Isolates a single embedded block: a bad diagram/widget shows an inline error
// instead of taking down the whole conversation. One per block (see markdown-block).
export class BlockErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <div className="my-1 rounded border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive">
          {this.props.label ?? 'Block'} failed to render: {this.state.error.message}
        </div>
      )
    }
    return this.props.children
  }
}

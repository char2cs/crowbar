import React from 'react'

interface Props {
  paneId: string
  children: React.ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class PaneBoundary extends React.Component<Props, State> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidUpdate(prevProps: Props) {
    if (prevProps.paneId !== this.props.paneId && this.state.hasError) {
      this.setState({ hasError: false, error: null })
    }
  }

  render() {
    if (this.state.hasError) {
      return (
        <PaneErrorState
          error={this.state.error}
          onRetry={() => this.setState({ hasError: false, error: null })}
        />
      )
    }
    return this.props.children
  }
}

function PaneErrorState({ error, onRetry }: { error: Error | null; onRetry: () => void }) {
  return (
    <div className="flex h-full w-full flex-col items-center justify-center gap-3 bg-pane-background p-8 text-center">
      <div className="text-sm font-medium text-destructive">This pane encountered an error</div>
      {error?.message && (
        <pre className="max-w-sm overflow-auto rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
          {error.message}
        </pre>
      )}
      <button
        type="button"
        onClick={onRetry}
        className="rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-accent-foreground hover:bg-accent/80"
      >
        Retry
      </button>
    </div>
  )
}

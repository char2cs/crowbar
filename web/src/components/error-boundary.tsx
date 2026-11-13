import { Component } from 'react'
import type { ReactNode, ErrorInfo } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

interface Props {
  children: ReactNode
  /** Drawn in place of the children; a function is handed the way to try them again. */
  fallback?: ReactNode | ((retry: () => void) => ReactNode)
  /** A change clears a caught error, so the children get another try. */
  resetKey?: unknown
}

interface State {
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[ErrorBoundary]', error, info)
  }

  componentDidUpdate(prev: Props) {
    if (this.state.error && prev.resetKey !== this.props.resetKey) this.reset()
  }

  reset = () => this.setState({ error: null })

  render() {
    if (this.state.error) {
      const { fallback } = this.props
      if (typeof fallback === 'function') return fallback(this.reset)
      if (fallback) return fallback
      return (
        <div className="flex flex-1 items-center justify-center p-8">
          <Card className="w-full max-w-sm border-destructive/20 bg-destructive/10">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm text-destructive">Something went wrong</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="font-mono text-[11px] text-muted-foreground">
                {this.state.error.message}
              </p>
              <Button variant="outline" size="sm" onClick={this.reset}>
                Try again
              </Button>
            </CardContent>
          </Card>
        </div>
      )
    }
    return this.props.children
  }
}

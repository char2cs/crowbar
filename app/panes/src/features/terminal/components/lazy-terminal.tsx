import { lazy, Suspense, type ComponentProps } from 'react'
import type { XtermTerminal as XtermTerminalComponent } from './terminal'

const XtermTerminal = lazy(() =>
  import('./terminal').then((module) => ({ default: module.XtermTerminal })),
)

export type XtermTerminalProps = ComponentProps<typeof XtermTerminalComponent>

export function LazyXtermTerminal(props: XtermTerminalProps) {
  return (
    <Suspense fallback={null}>
      <XtermTerminal {...props} />
    </Suspense>
  )
}

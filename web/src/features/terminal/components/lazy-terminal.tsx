import { lazy, Suspense, type ComponentProps } from 'react'
import type { XtermTerminal as XtermTerminalComponent } from './terminal'

// xterm and its addons (~600 KB raw) load the first time a terminal renders, not
// with the app shell. Everything outside the terminal feature renders this.
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

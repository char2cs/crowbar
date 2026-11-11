import type { ReactNode, Ref } from 'react'

export interface DockStackProps {
  /** The blank chat's document stands in for the dock: same slots, different frame. */
  blank: boolean
  dockRef: Ref<HTMLDivElement>
  /** Above the pill, tucked behind it: plan, queued messages, failures. */
  topTray?: ReactNode
  /** Command and file pickers that open over the pill. */
  pickers?: ReactNode
  /** Status lines before the pill: why the chat cannot take a prompt yet. */
  notes?: ReactNode
  /** The input box itself; it keeps its node, and its text, when the first send moves it down. */
  pill: ReactNode
  /** Below the pill, tucked behind it: subagents and shells. */
  bottomTray?: ReactNode
  /** The line under the pill: model, permission and context controls. */
  footer?: ReactNode
  /** A send-time error, centred under everything. */
  alert?: ReactNode
}

/** The slots around the composer, in a fixed order so no slot moves the pill's node. */
export function DockStack({
  blank,
  dockRef,
  topTray,
  pickers,
  notes,
  pill,
  bottomTray,
  footer,
  alert,
}: DockStackProps) {
  return (
    <div ref={blank ? undefined : dockRef} className={blank ? 'docframe' : 'dock'}>
      {topTray}
      {pickers}
      {notes}
      {pill}
      {bottomTray}
      {footer}
      {alert}
    </div>
  )
}

/** One entry per progressive-blur layer in the dock's dissolve (composer-dock.css). */
const DISSOLVE_LAYERS = [0, 1, 2, 3, 4, 5, 6]

/** Text scrolling under the dock blurs away instead of meeting a hard edge. */
export function DockDissolve() {
  return (
    <div className="dissolve" aria-hidden="true">
      {DISSOLVE_LAYERS.map((layer) => (
        <div key={layer} className="dissolve-layer" />
      ))}
    </div>
  )
}

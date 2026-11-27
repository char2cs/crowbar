import type { ReactNode, Ref } from 'react'

export interface DockStackProps {
  blank: boolean
  dockRef: Ref<HTMLDivElement>
  notes?: ReactNode
  pickers?: ReactNode
  topTray?: ReactNode
  pill: ReactNode
  bottomTray?: ReactNode
  footer?: ReactNode
  alert?: ReactNode
}

export function DockStack({
  blank,
  dockRef,
  notes,
  pickers,
  topTray,
  pill,
  bottomTray,
  footer,
  alert,
}: DockStackProps) {
  return (
    <div ref={blank ? undefined : dockRef} className={blank ? 'docframe' : 'dock'}>
      {notes}
      {pickers}
      {topTray}
      {pill}
      {bottomTray}
      {footer}
      {alert}
    </div>
  )
}

const DISSOLVE_LAYERS = [0, 1, 2, 3, 4, 5, 6]

export function DockDissolve() {
  return (
    <div className="dissolve" aria-hidden="true">
      {DISSOLVE_LAYERS.map((layer) => (
        <div key={layer} className="dissolve-layer" />
      ))}
    </div>
  )
}

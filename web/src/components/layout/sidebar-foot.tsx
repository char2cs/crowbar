import { useSidebar } from '@/components/ui/sidebar'
import { BuildReadout } from './build-readout'

/**
 * Fixed-height row at the bottom of the sidebar column. It anchors the readout
 * overlay, which takes no layout space; while the sidebar is hidden the tab
 * strip shows the readout instead, so exactly one is on screen.
 */
export function SidebarFoot() {
  const { open } = useSidebar()
  return <div className="relative h-10 shrink-0">{open && <BuildReadout className="left-3" />}</div>
}

import { useSidebar } from '@/components/ui/sidebar'
import { BuildReadout } from './build-readout'

// The sidebar's fixed-height foot, anchoring the readout overlay; while the sidebar is hidden the
// tab strip shows the readout instead, so exactly one is on screen.
export function SidebarFoot() {
  const { open } = useSidebar()
  return (
    <div className="relative h-10 shrink-0">
      {open && <BuildReadout className="left-3 right-3 top-auto bottom-0 w-auto" />}
    </div>
  )
}

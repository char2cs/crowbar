import { useRef } from 'react'
import { useSidebarWidth } from '@/hooks/useSidebarWidth'

interface AppShellProps {
  sidebar: React.ReactNode
  children: React.ReactNode
}

export function AppShell({ sidebar, children }: AppShellProps) {
  const { width, startResize } = useSidebarWidth()
  const dragging = useRef(false)

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      {/* Sidebar */}
      <div style={{ width }} className="relative flex-shrink-0 overflow-hidden">
        {sidebar}
        {/* Resize handle */}
        <div
          className="absolute inset-y-0 right-0 w-1 cursor-col-resize transition-colors hover:bg-primary/60"
          onMouseDown={e => { dragging.current = true; startResize(e) }}
        />
      </div>
      {/* Main */}
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden border-l border-border">
        {children}
      </div>
    </div>
  )
}

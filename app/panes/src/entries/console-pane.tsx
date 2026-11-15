import { useStore } from 'zustand'
import { ConsoleLog } from '@/features/console/components/console-log'
import { useConsoleStore } from '@/features/console/stores/console-store'

/** The daemon log filling the pane; the dock's position and size controls have no meaning in a tab. */
export function ConsolePane() {
  const entries = useStore(useConsoleStore, (s) => s.entries)
  const expandedId = useStore(useConsoleStore, (s) => s.expandedId)
  const toggleExpanded = useStore(useConsoleStore, (s) => s.toggleExpanded)
  return (
    <section
      aria-label="Console"
      className="bg-code text-code-foreground flex h-screen w-screen flex-col overflow-hidden"
    >
      <ConsoleLog entries={entries} expandedId={expandedId} onToggle={toggleExpanded} open />
    </section>
  )
}

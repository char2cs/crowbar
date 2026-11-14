import { SidebarFilesPanel } from '../sidebar-files-panel'

/** The phone IDE's file explorer: the desktop Files panel over the tabs;
 *  a picked file opens as an editor tab and the explorer steps aside for it. */
export function MobileFilesScreen({ onFileOpened }: { onFileOpened: () => void }) {
  return (
    <section aria-label="Files" className="absolute inset-0 z-10 flex flex-col bg-background">
      <SidebarFilesPanel onFileOpened={onFileOpened} />
    </section>
  )
}

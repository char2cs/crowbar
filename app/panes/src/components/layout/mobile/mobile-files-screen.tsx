import { SidebarFilesPanel } from '../sidebar-files-panel'

export function MobileFilesScreen({ onFileOpened }: { onFileOpened: () => void }) {
  return (
    <section aria-label="Files" className="absolute inset-0 z-10 flex flex-col bg-background">
      <SidebarFilesPanel onFileOpened={onFileOpened} />
    </section>
  )
}

import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { GitHistoryList } from './git-history-list'
import { GitChangesPanel } from './git-changes-panel'

export function GitPanel() {
  return (
    <Tabs defaultValue="changes" className="flex flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center px-2 py-1.5">
        <TabsList variant="default" className="w-full">
          <TabsTab value="changes" className="flex-1 justify-center">Changes</TabsTab>
          <TabsTab value="history" className="flex-1 justify-center">History</TabsTab>
        </TabsList>
      </div>

      <TabsPanel value="changes" className="flex flex-1 flex-col overflow-hidden">
        <GitChangesPanel />
      </TabsPanel>

      <TabsPanel value="history" className="flex flex-1 flex-col overflow-hidden">
        <GitHistoryList />
      </TabsPanel>
    </Tabs>
  )
}

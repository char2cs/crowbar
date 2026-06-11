import { GitPullRequest } from '@phosphor-icons/react'
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { openBranchReviewForActiveWorkspace } from '@/features/panes/utils/pane-command-actions'
import { GitHistoryList } from './git-history-list'
import { GitChangesPanel } from './git-changes-panel'

export function GitPanel() {
  return (
    <Tabs defaultValue="changes" className="flex flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center gap-1 px-2 py-1.5">
        <TabsList variant="default" className="min-w-0 flex-1">
          <TabsTab value="changes" className="flex-1 justify-center">
            Changes
          </TabsTab>
          <TabsTab value="history" className="flex-1 justify-center">
            History
          </TabsTab>
        </TabsList>
        <Button
          variant="ghost"
          size="icon-sm"
          className="shrink-0"
          title="Open Branch Review"
          aria-label="Open Branch Review"
          onClick={() => openBranchReviewForActiveWorkspace()}
        >
          <GitPullRequest />
        </Button>
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

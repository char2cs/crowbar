import { memo, type ReactNode } from "react";
// Database sidebars are out of scope — stubbed
const DatabaseSidebar = () => null
import { FileExplorerTree } from "@/features/file-explorer/components/file-explorer-tree";
import { useFileSystemStore } from "@/features/file-system/controllers/store";
import GitView from "@/features/git/components/git-view";
// GitHub PR view is out of scope — stubbed
const GitHubPRsView = () => null
import { SidebarPaneSelector } from "@/features/layout/components/sidebar/sidebar-pane-selector";
import { useSidebarPaneController } from "@/features/layout/hooks/use-sidebar-pane-controller";
import { getSidebarPaneLevel, type SidebarView } from "@/features/layout/utils/sidebar-pane-utils";
import { useSettingsStore } from "@/features/settings/store";
import { useSidebarStore } from "@/features/layout/stores/sidebar-store";
import { useBufferStore } from "@/features/editor/stores/buffer-store";
import { useUIState } from "@/features/window/stores/ui-state-store";
import { NotificationsPane } from "@/features/window/components/notifications-sidebar";
import { useExtensionViews } from "@/extensions/ui/hooks/use-extension-views";
import { ExtensionErrorBoundary } from "@/extensions/ui/components/extension-error-boundary";
import { LoadingSpinner } from "@/components/ui/spinner";
import { SidebarPanel } from "@/components/ui/sidebar";
import { cn } from "@/utils/cn";

interface MainSidebarProps {
  showActivityRail?: boolean;
  paneLevel?: "primary" | "edge";
  activeView?: SidebarView;
  isGitActive?: boolean;
  isGitHubPRsActive?: boolean;
  /** Optional children rendered below the sidebar content */
  children?: ReactNode;
}

interface SidebarPaneEntry {
  id: SidebarView;
  content: ReactNode;
}

export const SidebarActivityRail = memo(() => {
  const { isGitViewActive, isGitHubPRsViewActive, activeSidebarView } = useUIState();
  const openGlobalSearchBuffer = useBufferStore.use.actions().openGlobalSearchBuffer;
  const { settings } = useSettingsStore();
  const { openSidebarView } = useSidebarPaneController();

  const handleSidebarViewChange = (view: SidebarView | null) => {
    if (!view) return;
    openSidebarView(view, { triggerSide: "current" });
  };

  return (
    <div className="athas-sidebar-rail flex shrink-0 items-start px-1 pt-0 pb-1.5">
      <SidebarPaneSelector
        activeSidebarView={activeSidebarView ?? 'files'}
        isGitViewActive={isGitViewActive}
        isGitHubPRsViewActive={isGitHubPRsViewActive}
        coreFeatures={settings.coreFeatures}
        onViewChange={handleSidebarViewChange}
        onSearchClick={() => openGlobalSearchBuffer()}
        orientation="vertical"
      />
    </div>
  );
});

export const MainSidebar = memo(
  ({
    showActivityRail = true,
    paneLevel = "primary",
    activeView,
    isGitActive,
    isGitHubPRsActive,
    children,
  }: MainSidebarProps) => {
    const uiState = useUIState();
    const isGitViewActive = isGitActive ?? uiState.isGitViewActive;
    const isGitHubPRsViewActive = isGitHubPRsActive ?? uiState.isGitHubPRsViewActive;
    const activeSidebarView = activeView ?? uiState.activeSidebarView;
    const extensionViews = useExtensionViews();

    // file system store
    const setFiles = useFileSystemStore.use.setFiles?.();
    const handleCreateNewFolderInDirectory =
      useFileSystemStore.use.handleCreateNewFolderInDirectory?.();
    const handleFileSelect = useFileSystemStore.use.handleFileSelect?.();
    const handleFileOpen = useFileSystemStore.use.handleFileOpen?.();
    const handleCreateNewFileInDirectory =
      useFileSystemStore.use.handleCreateNewFileInDirectory?.();
    const handleDeletePath = useFileSystemStore.use.handleDeletePath?.();
    const refreshDirectory = useFileSystemStore.use.refreshDirectory?.();
    const handleFileMove = useFileSystemStore.use.handleFileMove?.();
    const handleRevealInFolder = useFileSystemStore.use.handleRevealInFolder?.();
    const handleDuplicatePath = useFileSystemStore.use.handleDuplicatePath?.();
    const handleRenamePath = useFileSystemStore.use.handleRenamePath?.();

    const rootFolderPath = useFileSystemStore.use.rootFolderPath?.();
    const files = useFileSystemStore.use.files();
    const isFileTreeLoading = useFileSystemStore.use.isFileTreeLoading();
    const isSwitchingProject = useFileSystemStore.use.isSwitchingProject();

    // sidebar store
    const activePath = useSidebarStore.use.activePath?.();
    const updateActivePath = useSidebarStore.use.updateActivePath?.();

    const { settings } = useSettingsStore();
    const showLeftSidebarTabs = settings.sidebarTabsPosition === "left";
    const shouldRenderActivityRail = showActivityRail && showLeftSidebarTabs;
    const activePaneId: SidebarView = isGitViewActive
      ? "git"
      : isGitHubPRsViewActive
        ? "github-prs"
        : (activeSidebarView ?? 'files');
    const allPaneEntries: SidebarPaneEntry[] = [
      ...(settings.coreFeatures.git
        ? [
            {
              id: "git" as const,
              content: (
                <GitView
                  repoPath={rootFolderPath ?? undefined}
                  onFileSelect={handleFileSelect ?? undefined}
                  isActive={isGitViewActive}
                />
              ),
            },
          ]
        : []),
      ...(settings.coreFeatures.github
        ? [
            {
              id: "github-prs" as const,
              content: <GitHubPRsView />,
            },
          ]
        : []),
      {
        id: "files",
        content: (
          <div className="relative h-full">
            {(!isFileTreeLoading || isSwitchingProject) && (
              <FileExplorerTree
                files={files}
                activePath={activePath ?? undefined}
                updateActivePath={updateActivePath ?? undefined}
                rootFolderPath={rootFolderPath ?? undefined}
                onFileSelect={handleFileSelect ?? (() => {})}
                onFileOpen={handleFileOpen ?? undefined}
                onCreateNewFileInDirectory={handleCreateNewFileInDirectory ?? ((_dir: string, _file: string) => {})}
                onCreateNewFolderInDirectory={handleCreateNewFolderInDirectory ?? undefined}
                onDeletePath={handleDeletePath ?? undefined}
                onUpdateFiles={setFiles ?? undefined}
                onRefreshDirectory={refreshDirectory ?? undefined}
                onRenamePath={handleRenamePath ?? undefined}
                onRevealInFinder={handleRevealInFolder ?? undefined}
                onFileMove={handleFileMove ?? undefined}
                onDuplicatePath={handleDuplicatePath ?? undefined}
              />
            )}

            {isFileTreeLoading && !isSwitchingProject && (
              <div className="pointer-events-none absolute inset-0 flex items-start justify-center p-3">
                <div className="rounded-full border border-border/60 bg-card/92 px-3 py-1.5 shadow-lg backdrop-blur-sm">
                  <LoadingSpinner label="Loading files" showLabel compact />
                </div>
              </div>
            )}
          </div>
        ),
      },
      {
        id: "notifications",
        content: <NotificationsPane />,
      },
      {
        id: "databases",
        content: <DatabaseSidebar />,
      },
      ...Array.from(extensionViews).map(
        ([viewId, view]) =>
          ({
            id: viewId,
            content: (
              <ExtensionErrorBoundary extensionId={view.extensionId} name={view.title}>
                {view.render()}
              </ExtensionErrorBoundary>
            ),
          }) satisfies SidebarPaneEntry,
      ),
    ];
    const paneEntries = allPaneEntries.filter(
      (pane) => pane.id === activeSidebarView || getSidebarPaneLevel(pane.id) === paneLevel,
    );
    const activePane = (() => {
      const requestedIndex = paneEntries.findIndex((pane) => pane.id === activePaneId);
      if (requestedIndex >= 0) return paneEntries[requestedIndex];

      return paneEntries[0] ?? null;
    })();
    return (
      <div className="flex h-full min-h-0" data-external-file-drop-scope="sidebar">
        {shouldRenderActivityRail ? <SidebarActivityRail /> : null}

        <SidebarPanel
          framed={shouldRenderActivityRail}
          className={cn(
            "min-w-0 flex-1 overflow-hidden",
            !shouldRenderActivityRail && "bg-transparent",
          )}
        >
          <div className="h-full min-h-0 overflow-hidden">{activePane?.content ?? null}</div>
          {children ?? null}
        </SidebarPanel>
      </div>
    );
  },
);

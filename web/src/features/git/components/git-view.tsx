import {
  ClockCounterClockwise,
  DotsThree as MoreHorizontal,
  FolderSimpleStar,
  GitBranch,
  ArrowClockwise as RefreshCw,
  TreeStructure,
} from "@phosphor-icons/react";
import { memo, type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSettingsStore } from "@/features/settings/store";
import { Button } from "@/components/ui/button";
import { LoadingSpinner } from "@/components/ui/loading-spinner";
import { PaneGroup } from "@/components/ui/pane";
import { primitiveAlert } from "@/components/ui/primitive-dialog-service";
import {
  SidebarEmptyActionState,
  SidebarEmptyState,
  SidebarFooter,
  SidebarHeader,
  SidebarHeaderIconButton,
  SidebarPanel,
  SidebarSectionPager,
  SidebarSectionSwitcher,
} from "@/components/ui/sidebar";
import { toast } from "@/components/ui/toast";
import { getBranches } from "../api/git-branches-api";
import { getGitLog } from "../api/git-commits-api";
import { clearRepositoryDiscoveryCache, resolveRepositoryPath } from "../api/git-repo-api";
import { getStashes } from "../api/git-stash-api";
import { getGitStatus, initRepository } from "../api/git-status-api";
import { openDirectory } from "@/lib/crowbar-bridge";
import { useGitDiffHandlers } from "../hooks/use-git-diff-handlers";
import { useGitFileDiffStats } from "../hooks/use-git-file-diff-stats";
import { useRepositoryStore } from "../stores/git-repository-store";
import { useGitStore } from "../stores/git-store";
import type { GitActionsMenuAnchorRect } from "../utils/git-actions-menu-position";
import { openGitWorktreeWorkspace } from "../utils/git-worktree-open";
import GitActionsMenu from "./git-actions-menu";
import GitBranchManager from "./git-branch-manager";
import GitCommitHistory from "./git-commit-history";
import GitCommitPanel from "./git-commit-panel";
import GitProjectSelector from "./git-project-selector";
import GitRemoteManager from "./git-remote-manager";
import { GitStashCommandSurface } from "./git-stash-command-surface";
import GitTagManager from "./git-tag-manager";
import GitWorktreeManager from "./git-worktree-manager";
import GitWorktreeSwitcher from "./git-worktree-switcher";
import GitStatusPanel from "./status/git-status-panel";

interface GitViewProps {
  repoPath?: string;
  onFileSelect?: (path: string, isDir: boolean) => void;
  isActive?: boolean;
}

type GitSidebarTab = "changes" | "history" | "worktrees";
type GitPaletteAction =
  | { type: "select-repository" }
  | { type: "show-tab"; tab: GitSidebarTab }
  | { type: "manage-remotes" }
  | { type: "manage-tags" }
  | { type: "view-stashes" }
  | { type: "refresh" };

const GitView = ({ repoPath, onFileSelect, isActive }: GitViewProps) => {
  const gitStatus = useGitStore((s) => s.gitStatus);
  const isLoadingGitData = useGitStore((s) => s.isLoadingGitData);
  const isRefreshing = useGitStore((s) => s.isRefreshing);
  const actions = useGitStore((s) => s.actions);
  const { setIsLoadingGitData, setIsRefreshing } = actions;
  const activeRepoPath = useRepositoryStore.use.activeRepoPath();
  const { syncWorkspaceRepositories, setManualRepository, refreshWorkspaceRepositories } =
    useRepositoryStore.use.actions();
  const [showGitActionsMenu, setShowGitActionsMenu] = useState(false);
  const [showStashList, setShowStashList] = useState(false);
  const [isSelectingRepo, setIsSelectingRepo] = useState(false);
  const [isInitializingRepo, setIsInitializingRepo] = useState(false);
  const [repoSelectionError, setRepoSelectionError] = useState<string | null>(null);
  const [gitActionsMenuAnchor, setGitActionsMenuAnchor] = useState<GitActionsMenuAnchorRect | null>(
    null,
  );

  const [showRemoteManager, setShowRemoteManager] = useState(false);
  const [showTagManager, setShowTagManager] = useState(false);
  const settings = useSettingsStore((s) => s.settings);
  const updateSetting = useSettingsStore((s) => s.updateSetting);
  const [activeTab, setActiveTab] = useState<GitSidebarTab>("changes");

  const wasActiveRef = useRef(isActive);

  const visibleGitFiles = useMemo(
    () =>
      settings.showUntrackedFiles
        ? (gitStatus?.files ?? [])
        : (gitStatus?.files ?? []).filter((file) => file.status !== "untracked"),
    [gitStatus?.files, settings.showUntrackedFiles],
  );

  const fileDiffStats = useGitFileDiffStats(activeRepoPath, visibleGitFiles);

  const { handleOpenOriginalFile, handleViewFileDiff, handleViewCommitDiff, handleViewStashDiff, handleViewTagComparison } =
    useGitDiffHandlers({ activeRepoPath, visibleGitFiles, onFileSelect });

  const handleSelectRepository = useCallback(async () => {
    setIsSelectingRepo(true);
    setRepoSelectionError(null);
    try {
      const selected = await openDirectory();

      if (!selected || Array.isArray(selected)) {
        return;
      }

      const resolvedRepoPath = await resolveRepositoryPath(selected);
      if (!resolvedRepoPath) {
        const message = "Selected folder is not inside a Git repository.";
        setRepoSelectionError(message);
        await primitiveAlert(message, "Select Repository");
        return;
      }

      setManualRepository(resolvedRepoPath);
    } catch (error) {
      console.error("Failed to select repository:", error);
      const message = "Failed to select repository";
      setRepoSelectionError(message);
      await primitiveAlert(`${message}:\n${error}`, "Select Repository");
    } finally {
      setIsSelectingRepo(false);
    }
  }, [setManualRepository]);

  const handleInitializeRepository = useCallback(async () => {
    const targetPath = repoPath;

    if (!targetPath) {
      toast.error("Open a folder before initializing a repository.");
      return;
    }

    setIsInitializingRepo(true);
    setRepoSelectionError(null);
    try {
      const success = await initRepository(targetPath);
      if (!success) {
        const message = "Failed to initialize repository.";
        setRepoSelectionError(message);
        toast.error(message);
        return;
      }

      clearRepositoryDiscoveryCache();
      setManualRepository(targetPath);
      await syncWorkspaceRepositories(targetPath, { force: true });
      window.dispatchEvent(new CustomEvent("git-status-changed"));
      toast.success("Repository initialized.");
    } catch (error) {
      console.error("Failed to initialize repository:", error);
      const message = error instanceof Error ? error.message : "Failed to initialize repository.";
      setRepoSelectionError(message);
      toast.error(message);
    } finally {
      setIsInitializingRepo(false);
    }
  }, [repoPath, setManualRepository, syncWorkspaceRepositories]);

  const loadInitialGitData = useCallback(async () => {
    if (!activeRepoPath) return;

    setIsLoadingGitData(true);
    try {
      const [status, commits, branches, stashes] = await Promise.all([
        getGitStatus(activeRepoPath),
        getGitLog(activeRepoPath, 50, 0),
        getBranches(activeRepoPath),
        getStashes(activeRepoPath),
      ]);

      actions.loadFreshGitData({
        gitStatus: status,
        commits,
        branches,
        stashes,
        repoPath: activeRepoPath,
      });
    } catch (error) {
      console.error("Failed to load initial git data:", error);
    } finally {
      setIsLoadingGitData(false);
    }
  }, [activeRepoPath, actions, setIsLoadingGitData]);

  const refreshGitData = useCallback(async () => {
    if (!activeRepoPath) return;

    try {
      const [status, branches, freshStashes] = await Promise.all([
        getGitStatus(activeRepoPath),
        getBranches(activeRepoPath),
        getStashes(activeRepoPath),
      ]);

      await actions.refreshGitData({
        gitStatus: status,
        branches,
        repoPath: activeRepoPath,
      });
      actions.setStashes(freshStashes);
    } catch (error) {
      console.error("Failed to refresh git data:", error);
    }
  }, [activeRepoPath, actions]);

  const handleManualRefresh = useCallback(async () => {
    setIsRefreshing(true);
    try {
      await Promise.all([
        refreshGitData(),
        refreshWorkspaceRepositories(),
        new Promise((resolve) => setTimeout(resolve, 500)),
      ]);
    } finally {
      setIsRefreshing(false);
    }
  }, [refreshGitData, refreshWorkspaceRepositories, setIsRefreshing]);

  const handleOpenWorktree = useCallback(
    async (worktreePath: string) => {
      const opened = await openGitWorktreeWorkspace(worktreePath);
      if (opened) {
        await refreshWorkspaceRepositories();
      }
    },
    [refreshWorkspaceRepositories],
  );

  const handleOpenWorktreeInNewWindow = useCallback(async (worktreePath: string) => {
    await openGitWorktreeWorkspace(worktreePath, { target: "new-window" });
  }, []);

  useEffect(() => {
    void syncWorkspaceRepositories(repoPath ?? null);
  }, [repoPath, syncWorkspaceRepositories]);

  useEffect(() => {
    loadInitialGitData();
  }, [loadInitialGitData]);

  useEffect(() => {
    setRepoSelectionError(null);
  }, [repoPath]);

  useEffect(() => {
    if (settings.autoRefreshGitStatus && isActive && !wasActiveRef.current && gitStatus) {
      refreshGitData();
    }
    wasActiveRef.current = isActive;
  }, [settings.autoRefreshGitStatus, isActive, gitStatus, refreshGitData]);

  useEffect(() => {
    if (!settings.autoRefreshGitStatus) return;

    const handleGitStatusChanged = () => {
      refreshGitData();
    };

    window.addEventListener("git-status-changed", handleGitStatusChanged);
    return () => {
      window.removeEventListener("git-status-changed", handleGitStatusChanged);
    };
  }, [settings.autoRefreshGitStatus, refreshGitData]);

  useEffect(() => {
    if (!settings.autoRefreshGitStatus) return;

    let refreshTimeout: NodeJS.Timeout | null = null;
    type FileExternalChangeDetail = {
      event_type: string;
      path: string;
    };

    const handleFileChange = (event: Event) => {
      if (!(event instanceof CustomEvent)) return;

      const { path } = event.detail as FileExternalChangeDetail;

      if (activeRepoPath && path.startsWith(activeRepoPath)) {
        if (refreshTimeout) {
          clearTimeout(refreshTimeout);
        }

        refreshTimeout = setTimeout(() => {
          refreshGitData();
        }, 300);
      }
    };

    window.addEventListener("file-external-change", handleFileChange);

    return () => {
      window.removeEventListener("file-external-change", handleFileChange);
      if (refreshTimeout) {
        clearTimeout(refreshTimeout);
      }
    };
  }, [settings.autoRefreshGitStatus, activeRepoPath, refreshGitData]);

  useEffect(() => {
    if (!settings.rememberLastGitPanelMode) return;
    setActiveTab(settings.gitLastPanelMode);
  }, [settings.rememberLastGitPanelMode, settings.gitLastPanelMode]);

  useEffect(() => {
    if (!settings.rememberLastGitPanelMode) return;
    if (settings.gitLastPanelMode !== activeTab) {
      void updateSetting("gitLastPanelMode", activeTab);
    }
  }, [activeTab, settings.rememberLastGitPanelMode, settings.gitLastPanelMode, updateSetting]);

  useEffect(() => {
    const handlePaletteAction = (event: Event) => {
      if (!(event instanceof CustomEvent)) return;

      const detail = event.detail as GitPaletteAction;
      if (!detail) return;

      if (detail.type === "select-repository") {
        void handleSelectRepository();
        return;
      }

      if (detail.type === "show-tab") {
        setActiveTab(detail.tab);
        return;
      }

      if (detail.type === "manage-remotes") {
        setShowRemoteManager(true);
        return;
      }

      if (detail.type === "manage-tags") {
        setShowTagManager(true);
        return;
      }

      if (detail.type === "view-stashes") {
        setShowStashList(true);
        return;
      }

      if (detail.type === "refresh") {
        void handleManualRefresh();
      }
    };

    window.addEventListener("athas:git-palette-action", handlePaletteAction);
    return () => window.removeEventListener("athas:git-palette-action", handlePaletteAction);
  }, [handleManualRefresh, handleSelectRepository]);

  const renderActionsButton = () => (
    <SidebarHeaderIconButton
      onClick={(e) => {
        const rect = e.currentTarget.getBoundingClientRect();
        setGitActionsMenuAnchor({
          left: rect.left,
          right: rect.right,
          top: rect.top,
          bottom: rect.bottom,
          width: rect.width,
          height: rect.height,
        });
        setShowGitActionsMenu(!showGitActionsMenu);
        setShowStashList(false);
      }}
      tooltip="Git Actions"
    >
      <MoreHorizontal />
    </SidebarHeaderIconButton>
  );

  const renderInitializeRepositoryButton = () => {
    const canInitializeRepository = Boolean(repoPath);

    return (
      <Button
        onClick={() => void handleInitializeRepository()}
        disabled={!canInitializeRepository || isInitializingRepo}
        variant="secondary"
        compact
        className="mt-1.5 h-6 px-2 ui-text-xs"
        tooltip={
          canInitializeRepository
            ? "Initialize Git repository"
            : "Open a folder before initializing Git"
        }
      >
        <GitBranch />
        {isInitializingRepo ? "Initializing..." : "Initialize Repository"}
      </Button>
    );
  };

  const renderGitActionsMenu = ({
    hasGitRepo,
    onRefresh,
  }: {
    hasGitRepo: boolean;
    onRefresh?: () => void;
  }) => (
    <GitActionsMenu
      isOpen={showGitActionsMenu}
      anchorRect={gitActionsMenuAnchor}
      onClose={() => {
        setShowGitActionsMenu(false);
        setGitActionsMenuAnchor(null);
      }}
      hasGitRepo={hasGitRepo}
      repoPath={activeRepoPath ?? repoPath}
      onRefresh={onRefresh}
      onOpenRemoteManager={() => setShowRemoteManager(true)}
      onOpenTagManager={() => setShowTagManager(true)}
      onViewStashes={() => setShowStashList(true)}
      onSelectRepository={handleSelectRepository}
      isSelectingRepository={isSelectingRepo}
      onInitializeRepository={handleInitializeRepository}
      isInitializingRepository={isInitializingRepo}
    />
  );

  const gitTabOrder: GitSidebarTab[] = ["changes", "history", "worktrees"];
  const gitTabs: Array<{
    id: GitSidebarTab;
    label: string;
    icon: ReactNode;
  }> = [...settings.gitSidebarTabOrder]
    .sort((a, b) => gitTabOrder.indexOf(a) - gitTabOrder.indexOf(b))
    .map((id) => {
      const tabMap: Record<GitSidebarTab, { id: GitSidebarTab; label: string; icon: ReactNode }> = {
        changes: {
          id: "changes",
          label: "Changes",
          icon: <FolderSimpleStar size={16} weight="duotone" />,
        },
        history: {
          id: "history",
          label: "History",
          icon: <ClockCounterClockwise size={16} weight="duotone" />,
        },
        worktrees: {
          id: "worktrees",
          label: "Worktrees",
          icon: <TreeStructure size={16} weight="duotone" />,
        },
      };

      return tabMap[id];
    })
    .filter(Boolean);

  if (!activeRepoPath) {
    return (
      <>
        <SidebarPanel className="gap-2 p-2">
          <SidebarHeader className="justify-between bg-transparent px-0 py-0 backdrop-blur-none">
            <div className="flex items-center gap-2">{renderActionsButton()}</div>
          </SidebarHeader>
          <SidebarEmptyActionState
            className="h-full"
            message="No repository selected"
            actionLabel={isSelectingRepo ? "Selecting..." : "Browse Repository"}
            actionDisabled={isSelectingRepo}
            onAction={() => void handleSelectRepository()}
          >
            {renderInitializeRepositoryButton()}
            {repoSelectionError ? (
              <span className="ui-text-sm mt-1.5 text-red-400">{repoSelectionError}</span>
            ) : null}
          </SidebarEmptyActionState>
        </SidebarPanel>
        {renderGitActionsMenu({ hasGitRepo: false, onRefresh: handleManualRefresh })}
      </>
    );
  }

  if (isLoadingGitData && !gitStatus) {
    return (
      <>
        <SidebarPanel className="gap-2 p-2">
          <SidebarHeader className="justify-between bg-transparent px-0 py-0 backdrop-blur-none">
            <div className="flex items-center gap-2">{renderActionsButton()}</div>
          </SidebarHeader>
          <SidebarEmptyState className="h-full">Loading Git status...</SidebarEmptyState>
        </SidebarPanel>
        {renderGitActionsMenu({ hasGitRepo: false, onRefresh: handleManualRefresh })}
      </>
    );
  }

  if (!gitStatus) {
    return (
      <>
        <SidebarPanel className="gap-2 p-2">
          <SidebarHeader className="justify-between bg-transparent px-0 py-0 backdrop-blur-none">
            <div className="flex items-center gap-2">{renderActionsButton()}</div>
          </SidebarHeader>
          <SidebarEmptyActionState
            className="h-full"
            message="Not a Git repository"
            actionLabel={isSelectingRepo ? "Selecting..." : "Browse Repository"}
            actionDisabled={isSelectingRepo}
            onAction={() => void handleSelectRepository()}
          >
            {renderInitializeRepositoryButton()}
            {repoSelectionError ? (
              <span className="ui-text-sm mt-1.5 text-red-400">{repoSelectionError}</span>
            ) : null}
          </SidebarEmptyActionState>
        </SidebarPanel>
        {renderGitActionsMenu({ hasGitRepo: false, onRefresh: handleManualRefresh })}
      </>
    );
  }

  const stagedFiles = visibleGitFiles.filter((f) => f.staged);
  const refreshAfterAction = settings.autoRefreshGitStatus ? handleManualRefresh : undefined;
  const handleGitFileClick = settings.openDiffOnClick ? handleViewFileDiff : handleOpenOriginalFile;

  return (
    <>
      <SidebarPanel className="ui-font ui-text-sm select-none gap-2 p-2">
        <SidebarHeader className="min-w-0 bg-transparent px-0 py-0 backdrop-blur-none">
          <PaneGroup className="min-w-0 flex-1 overflow-hidden">
            <GitProjectSelector
              className="shrink"
              onRepositoryChange={() => setRepoSelectionError(null)}
            />
            <GitBranchManager
              currentBranch={gitStatus.branch}
              repoPath={activeRepoPath}
              onBranchChange={refreshAfterAction}
              triggerClassName="shrink"
            />
            <GitWorktreeSwitcher
              repoPath={activeRepoPath}
              triggerClassName="shrink"
              triggerInputClassName="max-w-[120px]"
              onWorktreeChange={(worktreePath) => void handleOpenWorktree(worktreePath)}
            />
          </PaneGroup>

          <div className="flex shrink-0 items-center gap-1">
            <SidebarHeaderIconButton
              onClick={handleManualRefresh}
              disabled={isLoadingGitData || isRefreshing}
              className="disabled:opacity-50"
              tooltip="Refresh"
              aria-label="Refresh git status"
            >
              {isLoadingGitData || isRefreshing ? (
                <LoadingSpinner label="Refreshing git status" compact />
              ) : (
                <RefreshCw />
              )}
            </SidebarHeaderIconButton>
            {renderActionsButton()}
          </div>
        </SidebarHeader>

        <div className="@container flex min-h-0 flex-1 flex-col gap-2 overflow-hidden">
          <SidebarSectionSwitcher
            items={gitTabs}
            value={activeTab}
            onChange={(tab) => setActiveTab(tab as GitSidebarTab)}
          />

          <SidebarSectionPager
            className="flex-1"
            items={[
              {
                id: "changes",
                content: (
                  <GitStatusPanel
                    files={visibleGitFiles}
                    fileDiffStats={fileDiffStats}
                    onFileSelect={handleGitFileClick}
                    onOpenFile={handleOpenOriginalFile}
                    onRefresh={refreshAfterAction}
                    repoPath={activeRepoPath}
                  />
                ),
              },
              {
                id: "history",
                content: (
                  <GitCommitHistory
                    isCollapsed={false}
                    onToggle={() => {}}
                    onViewCommitDiff={handleViewCommitDiff}
                    repoPath={activeRepoPath}
                    showHeader={false}
                  />
                ),
              },
              {
                id: "worktrees",
                content: (
                  <GitWorktreeManager
                    embedded
                    repoPath={activeRepoPath}
                    onRefresh={refreshAfterAction}
                    onSelectWorktree={handleOpenWorktree}
                    onOpenWorktreeInNewWindow={handleOpenWorktreeInNewWindow}
                  />
                ),
              },
            ].filter((item) => gitTabs.some((tab) => tab.id === item.id))}
            value={activeTab}
            onChange={(tab) => setActiveTab(tab as GitSidebarTab)}
          />
        </div>

        <SidebarFooter surface>
          <GitCommitPanel
            stagedFilesCount={stagedFiles.length}
            repoPath={activeRepoPath}
            ahead={gitStatus.ahead}
            behind={gitStatus.behind}
            onCommitSuccess={refreshAfterAction}
          />
        </SidebarFooter>
      </SidebarPanel>

      {renderGitActionsMenu({ hasGitRepo: !!gitStatus, onRefresh: refreshAfterAction })}
      <GitStashCommandSurface
        isOpen={showStashList}
        onClose={() => setShowStashList(false)}
        repoPath={activeRepoPath}
        onRefresh={refreshAfterAction}
        onViewStashDiff={handleViewStashDiff}
      />

      <GitRemoteManager
        isOpen={showRemoteManager}
        onClose={() => setShowRemoteManager(false)}
        repoPath={activeRepoPath}
        onRefresh={refreshAfterAction}
      />

      <GitTagManager
        isOpen={showTagManager}
        onClose={() => setShowTagManager(false)}
        repoPath={activeRepoPath}
        onRefresh={refreshAfterAction}
        onViewTagComparison={handleViewTagComparison}
      />
    </>
  );
};

export default memo(GitView);

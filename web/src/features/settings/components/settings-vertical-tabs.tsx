import {
  Bug,
  CodeBlock,
  GitBranch,
  PaintBrush,
  TerminalWindow,
  TreeStructure,
} from "@phosphor-icons/react";
import * as React from "react";
import { useSettingsStore } from "@/features/settings/store";
import { filterVisibleSettingsTabs } from "@/features/settings/lib/settings-tab-visibility";
import { useAuthStore } from "@/features/window/stores/auth-store";
import type { SettingsTab } from "@/features/window/stores/ui-state-store";
import { Button } from "@/components/ui/button";
import { cn } from "@/utils/cn";

interface SettingsVerticalTabsProps {
  activeTab: SettingsTab;
  onTabChange: (tab: SettingsTab) => void;
  panelIdForTab?: (tab: SettingsTab) => string;
}

export interface SettingsTabItem {
  id: SettingsTab;
  label: string;
  icon: React.ComponentType<{
    size?: string | number;
    className?: string;
    weight?: "regular" | "duotone";
  }>;
}

export const SETTINGS_TAB_ITEMS: SettingsTabItem[] = [
  { id: "appearance",    label: "Appearance",  icon: PaintBrush },
  { id: "editor",        label: "Editor",      icon: CodeBlock },
  { id: "file-explorer", label: "Files",       icon: TreeStructure },
  { id: "git",           label: "Git",         icon: GitBranch },
  { id: "terminal",      label: "Terminal",    icon: TerminalWindow },
  ...(import.meta.env.DEV ? [{ id: "developer" as SettingsTabItem["id"], label: "Developer", icon: Bug }] : []),
];

export const SettingsVerticalTabs = ({
  activeTab,
  onTabChange,
  panelIdForTab = (tab) => `settings-panel-${tab}`,
}: SettingsVerticalTabsProps) => {
  const searchQuery = useSettingsStore((state) => state.search.query);
  const searchResults = useSettingsStore((state) => state.search.results);
  const subscription = useAuthStore((state) => state.subscription);
  const hasEnterpriseAccess = Boolean(subscription?.enterprise?.has_access);
  const hasTeamsAccess = Boolean(subscription?.collaboration?.enabled);
  const scrollContainerRef = React.useRef<HTMLDivElement>(null);
  const tabRefs = React.useRef<Array<HTMLButtonElement | null>>([]);

  const matchingTabs = searchQuery ? new Set(searchResults.map((result) => result.tab)) : null;

  const visibleTabs = filterVisibleSettingsTabs(SETTINGS_TAB_ITEMS, {
    hasEnterpriseAccess,
    hasTeamsAccess,
    matchingTabs,
  });

  React.useEffect(() => {
    if (searchQuery && visibleTabs.length > 0) {
      const firstVisibleTab = visibleTabs[0].id;
      if (firstVisibleTab !== activeTab) {
        onTabChange(firstVisibleTab);
      }
    }
  }, [searchQuery, visibleTabs, activeTab, onTabChange]);

  const handleSidebarWheel = (event: React.WheelEvent<HTMLDivElement>) => {
    const container = scrollContainerRef.current;
    if (!container) return;

    const canScroll = container.scrollHeight > container.clientHeight;
    if (!canScroll || event.deltaY === 0) return;

    container.scrollTop += event.deltaY;
    event.preventDefault();
  };

  const focusTabAtIndex = React.useCallback(
    (index: number) => {
      const nextTab = visibleTabs[index];
      if (!nextTab) return;

      onTabChange(nextTab.id);
      window.requestAnimationFrame(() => {
        tabRefs.current[index]?.focus();
      });
    },
    [onTabChange, visibleTabs],
  );

  return (
    <div className="flex h-full flex-col">
      <div
        ref={scrollContainerRef}
        role="tablist"
        aria-orientation="vertical"
        aria-label="Settings sections"
        className="flex-1 space-y-0.5 overflow-hidden p-2"
        onWheelCapture={handleSidebarWheel}
      >
        {visibleTabs.length > 0 ? (
          visibleTabs.map((item, index) => {
            const Icon = item.icon;
            const isActive = activeTab === item.id;

            return (
              <Button
                key={item.id}
                ref={(element) => {
                  tabRefs.current[index] = element;
                }}
                type="button"
                variant="ghost"
                compact
                onClick={() => onTabChange(item.id)}
                onKeyDown={(event) => {
                  switch (event.key) {
                    case "ArrowDown":
                    case "ArrowRight":
                      event.preventDefault();
                      focusTabAtIndex((index + 1) % visibleTabs.length);
                      break;
                    case "ArrowUp":
                    case "ArrowLeft":
                      event.preventDefault();
                      focusTabAtIndex((index - 1 + visibleTabs.length) % visibleTabs.length);
                      break;
                    case "Home":
                      event.preventDefault();
                      focusTabAtIndex(0);
                      break;
                    case "End":
                      event.preventDefault();
                      focusTabAtIndex(visibleTabs.length - 1);
                      break;
                    default:
                      break;
                  }
                }}
                role="tab"
                id={`settings-tab-${item.id}`}
                aria-selected={isActive}
                aria-controls={panelIdForTab(item.id)}
                tabIndex={isActive ? 0 : -1}
                className={cn(
                  "ui-text-sm h-auto w-full justify-start gap-2.5 rounded-xl px-2.5 py-1.5 text-left",
                  isActive
                    ? "bg-accent/10 text-accent-foreground font-medium"
                    : "text-muted-foreground hover:bg-accent/5 hover:text-foreground",
                )}
              >
                <Icon className="size-[18px] shrink-0 text-current" weight="duotone" />
                <span className="truncate">{item.label}</span>
              </Button>
            );
          })
        ) : (
          <div className="ui-font ui-text-sm p-2 text-center text-muted-foreground">
            No matching settings
          </div>
        )}
      </div>

    </div>
  );
};

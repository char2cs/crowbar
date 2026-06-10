import { Archive, Download, Trash as Trash2, Upload } from "@phosphor-icons/react";
import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { CommandEmpty, CommandList } from "@/components/ui/command";
import { formatRelativeDate } from "@/utils/date";
import { matchesSearchQuery } from "@/utils/search-match";
import { applyStash, dropStash, getStashes, popStash } from "../api/git-stash-api";
import { useGitStore } from "../stores/git-store";
import { getStashDisplayTitle, getStashPositionLabel } from "../utils/git-stash-format";
import GitCommandSurface from "./git-command-surface";

interface GitStashCommandSurfaceProps {
  isOpen: boolean;
  onClose: () => void;
  repoPath: string | null;
  onRefresh?: () => Promise<void> | void;
  onViewStashDiff: (stashIndex: number) => Promise<void>;
}

export function GitStashCommandSurface({
  isOpen,
  onClose,
  repoPath,
  onRefresh,
  onViewStashDiff,
}: GitStashCommandSurfaceProps) {
  const stashes = useGitStore((s) => s.stashes);
  const setStashes = useGitStore((s) => s.actions.setStashes);
  const [searchQuery, setSearchQuery] = useState("");
  const [actionLoading, setActionLoading] = useState<Set<number>>(new Set());

  // Hooks below must run unconditionally; the empty-repoPath bail-out happens
  // after them (was an early return above useMemo — a rules-of-hooks violation
  // that crashed the surface when repoPath toggled between renders).
  const filteredStashes = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) return stashes;
    return stashes.filter((stash) =>
      matchesSearchQuery(query, [
        getStashDisplayTitle(stash.message),
        getStashPositionLabel(stash.index),
        `stash ${stash.index + 1}`,
        `stash@{${stash.index}}`,
      ]),
    );
  }, [searchQuery, stashes]);

  if (!repoPath) return null;

  const handleClose = () => {
    onClose();
    setSearchQuery("");
  };

  const handleStashAction = async (
    action: () => Promise<boolean>,
    stashIndex: number,
    actionName: string,
  ) => {
    setActionLoading((prev) => new Set(prev).add(stashIndex));
    try {
      const success = await action();
      if (success) {
        if (onRefresh) {
          await onRefresh();
        } else if (repoPath) {
          setStashes(await getStashes(repoPath));
        }
      } else {
        console.error(`${actionName} failed`);
      }
    } catch (error) {
      console.error(`${actionName} error:`, error);
    } finally {
      setActionLoading((prev) => {
        const next = new Set(prev);
        next.delete(stashIndex);
        return next;
      });
    }
  };

  return (
    <GitCommandSurface
      isOpen={isOpen}
      onClose={handleClose}
      query={searchQuery}
      onQueryChange={setSearchQuery}
      placeholder="Search stashes..."
      meta={`${stashes.length} stash${stashes.length === 1 ? "" : "es"}`}
    >
      <CommandList>
        {filteredStashes.length === 0 ? (
          <CommandEmpty>{searchQuery.trim() ? "No matching stashes" : "No stashes"}</CommandEmpty>
        ) : (
          filteredStashes.map((stash) => {
            const displayTitle = getStashDisplayTitle(stash.message);
            const isActionLoading = actionLoading.has(stash.index);

            return (
              <div
                key={stash.index}
                role="button"
                tabIndex={0}
                className="group/stash ui-font relative mb-1 flex min-h-12 w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-left transition-colors hover:bg-muted focus:bg-muted focus:outline-none"
                onClick={() => {
                  void onViewStashDiff(stash.index);
                  handleClose();
                }}
                onKeyDown={(event) => {
                  if (event.target !== event.currentTarget) return;
                  if (event.key !== "Enter" && event.key !== " ") return;
                  event.preventDefault();
                  void onViewStashDiff(stash.index);
                  handleClose();
                }}
              >
                <div className="flex size-7 shrink-0 items-center justify-center rounded-md border border-border/50 bg-card/70 text-muted-foreground">
                  <Archive className="size-4" />
                </div>
                <div className="min-w-0 flex-1 pr-24">
                  <div className="ui-text-sm truncate text-foreground" title={displayTitle}>
                    {displayTitle}
                  </div>
                  <div className="ui-text-xs mt-1 flex min-w-0 items-center gap-2 text-muted-foreground/80">
                    <span className="truncate">{formatRelativeDate(stash.date)}</span>
                    <span className="rounded border border-border/50 px-1 ui-text-xs leading-4">
                      {getStashPositionLabel(stash.index)}
                    </span>
                  </div>
                </div>
                <div className="pointer-events-none absolute right-2 top-1/2 flex -translate-y-1/2 translate-x-1 items-center gap-0.5 rounded-md border border-border/60 bg-card p-0.5 opacity-0 transition-all group-hover/stash:pointer-events-auto group-hover/stash:translate-x-0 group-hover/stash:opacity-100 group-focus-within/stash:pointer-events-auto group-focus-within/stash:translate-x-0 group-focus-within/stash:opacity-100">
                  <Button
                    type="button"
                    onClick={(event) => {
                      event.stopPropagation();
                      void handleStashAction(
                        () => applyStash(repoPath, stash.index),
                        stash.index,
                        "Apply stash",
                      );
                    }}
                    disabled={isActionLoading}
                    variant="ghost"
                    compact
                    className="text-muted-foreground disabled:opacity-50"
                    tooltip="Apply stash"
                  >
                    <Download />
                  </Button>
                  <Button
                    type="button"
                    onClick={(event) => {
                      event.stopPropagation();
                      void handleStashAction(
                        () => popStash(repoPath, stash.index),
                        stash.index,
                        "Pop stash",
                      );
                    }}
                    disabled={isActionLoading}
                    variant="ghost"
                    compact
                    className="text-muted-foreground disabled:opacity-50"
                    tooltip="Pop stash"
                  >
                    <Upload />
                  </Button>
                  <Button
                    type="button"
                    onClick={(event) => {
                      event.stopPropagation();
                      void handleStashAction(
                        () => dropStash(repoPath, stash.index),
                        stash.index,
                        "Drop stash",
                      );
                    }}
                    disabled={isActionLoading}
                    variant="ghost"
                    compact
                    className="text-red-400 hover:bg-red-900/20 hover:text-red-300 disabled:opacity-50"
                    tooltip="Drop stash"
                  >
                    <Trash2 />
                  </Button>
                </div>
              </div>
            );
          })
        )}
      </CommandList>
    </GitCommandSurface>
  );
}

import { useCallback, useEffect, useRef, useState } from "react";
import type { CompletionItem } from "vscode-languageserver-protocol";
import { extensionRegistry } from "@/extensions/registry/extension-registry";
import { editorAPI } from "@/features/editor/extensions/api";
import {
  expandSnippet,
  getCurrentTabStop,
  nextTabStop,
  previousTabStop,
} from "@/features/editor/snippets/snippet-expander";
import type { SnippetSession } from "@/features/editor/snippets/types";
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";
import { getActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";
import { findPaneGroup } from "@/features/panes/utils/pane-tree";
import { useEditorStateStore } from "@/features/editor/stores/state-store";
import type { Position } from "@/features/editor/types/editor";
import { calculateCursorPositionFromContent } from "@/features/editor/utils/position";
import { isEditorContent } from "@/features/panes/types/pane-content";
import { logger } from "@/features/editor/utils/logger";

/**
 * Hook for snippet completion integration
 * Provides snippet completions and handles expansion/navigation
 */
export function useSnippetCompletion(filePath: string | undefined) {
  const workspaceStore = useWorkspaceStore();
  const [activeSession, setActiveSession] = useState<SnippetSession | null>(null);
  const sessionRef = useRef<SnippetSession | null>(null);

  // Sync ref with state
  useEffect(() => {
    sessionRef.current = activeSession;
  }, [activeSession]);

  /**
   * Get snippet completions for the current file/language
   */
  const getSnippetCompletions = useCallback(
    (prefix: string): CompletionItem[] => {
      if (!filePath) return [];

      const languageId = extensionRegistry.getLanguageId(filePath);
      if (!languageId) return [];

      const snippets = extensionRegistry.getSnippetsForLanguage(languageId);

      // Filter by prefix
      const matchingSnippets = snippets.filter((snippet) =>
        snippet.prefix.toLowerCase().startsWith(prefix.toLowerCase()),
      );

      // Convert to LSP CompletionItem format
      return matchingSnippets.map((snippet) => ({
        label: snippet.prefix,
        kind: 15, // CompletionItemKind.Snippet
        detail: snippet.description || "Snippet",
        insertText: Array.isArray(snippet.body) ? snippet.body.join("\n") : snippet.body,
        insertTextFormat: 2, // InsertTextFormat.Snippet
        data: {
          isSnippet: true,
          snippet,
        },
      }));
    },
    [filePath],
  );

  /**
   * Expand a snippet at the current cursor position
   */
  const expandSnippetAtCursor = useCallback((completion: CompletionItem) => {
    const wsState = workspaceStore.getState();
    const activeBufferId = findPaneGroup(wsState.paneRoot, wsState.activePaneId)?.activeBufferId ?? null;
    if (!activeBufferId || !completion.data?.isSnippet) return false;

    const cursorPosition = useEditorStateStore.getState().cursorPosition;
    const buffer = wsState.buffers.find((b) => b.id === activeBufferId);
    if (!buffer || !isEditorContent(buffer)) return false;

    const snippet = completion.data.snippet;
    if (!snippet) return false;

    try {
      // Create snippet session
      const session = expandSnippet(
        { body: snippet.body, prefix: snippet.prefix },
        cursorPosition,
        {
          fileName: buffer.name,
          filePath: buffer.path,
        },
      );

      const content = buffer.content;

      // Remove the trigger prefix
      const beforeCursor = content.slice(0, cursorPosition.offset);
      const afterCursor = content.slice(cursorPosition.offset);
      const lastWord = beforeCursor.match(/(\w+)$/);
      const prefixLength = lastWord ? lastWord[0].length : 0;

      const newContent =
        content.slice(0, cursorPosition.offset - prefixLength) +
        session.parsedSnippet.expandedBody +
        afterCursor;

      workspaceStore.setState((state) => ({
        buffers: state.buffers.map((b) =>
          b.id === activeBufferId && b.type === "editor"
            ? { ...b, content: newContent, isDirty: true }
            : b,
        ),
      }));

      // If snippet has tab stops, activate session
      if (session.parsedSnippet.hasTabStops) {
        setActiveSession(session);

        // Move to first tab stop
        const firstTabStop = getCurrentTabStop(session);
        if (firstTabStop) {
          const newOffset = cursorPosition.offset - prefixLength + firstTabStop.offset;
          const newPosition = calculatePosition(newContent, newOffset);

          useEditorStateStore
            .getState()
            .actions.setCursorPosition(newPosition, { ensureVisible: false });
          editorAPI.setCursorPosition(newPosition);

          // Select placeholder if it exists
          if (firstTabStop.placeholder && firstTabStop.length > 0) {
            selectRange(newOffset, newOffset + firstTabStop.length);
          }
        }
      }

      logger.info("SnippetCompletion", `Expanded snippet: ${snippet.prefix}`);
      return true;
    } catch (error) {
      logger.error("SnippetCompletion", "Failed to expand snippet:", error);
      return false;
    }
  }, []);

  /**
   * Navigate to next tab stop
   */
  const jumpToNextTabStop = useCallback(() => {
    if (!sessionRef.current || !sessionRef.current.isActive) return false;

    const tabStop = nextTabStop(sessionRef.current);

    if (!tabStop) {
      // No more tab stops, end session
      setActiveSession(null);
      return false;
    }

    // Calculate absolute position for this tab stop
    const session = sessionRef.current;
    const newOffset = session.insertPosition.offset + tabStop.offset;
    const wsStateNext = workspaceStore.getState();
    const nextActiveId = findPaneGroup(wsStateNext.paneRoot, wsStateNext.activePaneId)?.activeBufferId ?? null;
    const buffer = nextActiveId ? wsStateNext.buffers.find((b) => b.id === nextActiveId) : undefined;

    if (buffer && isEditorContent(buffer)) {
      const newPosition = calculatePosition(buffer.content, newOffset);
      useEditorStateStore
        .getState()
        .actions.setCursorPosition(newPosition, { ensureVisible: false });
      editorAPI.setCursorPosition(newPosition);

      // Select placeholder if it exists
      if (tabStop.placeholder && tabStop.length > 0) {
        selectRange(newOffset, newOffset + tabStop.length);
      }
    }

    return true;
  }, []);

  /**
   * Navigate to previous tab stop
   */
  const jumpToPreviousTabStop = useCallback(() => {
    if (!sessionRef.current || !sessionRef.current.isActive) return false;

    const tabStop = previousTabStop(sessionRef.current);

    if (!tabStop) return false;

    // Calculate absolute position for this tab stop
    const session = sessionRef.current;
    const newOffset = session.insertPosition.offset + tabStop.offset;
    const wsStatePrev = workspaceStore.getState();
    const prevActiveId = findPaneGroup(wsStatePrev.paneRoot, wsStatePrev.activePaneId)?.activeBufferId ?? null;
    const buffer = prevActiveId ? wsStatePrev.buffers.find((b) => b.id === prevActiveId) : undefined;

    if (buffer && isEditorContent(buffer)) {
      const newPosition = calculatePosition(buffer.content, newOffset);
      useEditorStateStore
        .getState()
        .actions.setCursorPosition(newPosition, { ensureVisible: false });

      // Select placeholder if it exists
      if (tabStop.placeholder && tabStop.length > 0) {
        selectRange(newOffset, newOffset + tabStop.length);
      }
    }

    return true;
  }, []);

  /**
   * Exit snippet mode
   */
  const exitSnippetMode = useCallback(() => {
    if (sessionRef.current) {
      sessionRef.current.isActive = false;
      setActiveSession(null);
    }
  }, []);

  return {
    getSnippetCompletions,
    expandSnippetAtCursor,
    jumpToNextTabStop,
    jumpToPreviousTabStop,
    exitSnippetMode,
    hasActiveSession: activeSession !== null,
    activeSession,
  };
}

/**
 * Calculate position from offset
 */
function calculatePosition(content: string, offset: number): Position {
  return calculateCursorPositionFromContent(offset, content);
}

/**
 * Select a range in the textarea
 */
function selectRange(start: number, end: number) {
  const store = getActiveWorkspaceStoreRef();
  if (!store) return;
  const state = store.getState();
  const activeBufferId = findPaneGroup(state.paneRoot, state.activePaneId)?.activeBufferId ?? null;
  const activeBuffer = activeBufferId ? state.buffers.find((b) => b.id === activeBufferId) : null;
  if (!activeBuffer || !isEditorContent(activeBuffer)) return;

  editorAPI.setSelection({
    start: calculatePosition(activeBuffer.content, start),
    end: calculatePosition(activeBuffer.content, end),
  });
}

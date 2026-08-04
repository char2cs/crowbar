//! `getExplorerTargetPath` — the one live export of `web/src/features/
//! file-explorer/file-explorer/utils/file-explorer-tree-utils.ts`.
//!
//! # Why the file's other four exports are not here
//!
//! `native/mapping/tier-a-denominator.md` §8's export-level audit found
//! `filterHiddenFiles`, `addNewItemToTree` and `removeEditingItemsFromTree`
//! dead (zero non-test references anywhere; each one's only self-file hit is
//! pure recursion, not a real caller) and the file's own exported
//! `getAncestorDirectoryPaths` test-only (exercised by
//! `file-explorer-tree-utils.test.ts` but never called in production —
//! `file-tree-gitignore.ts` has its own independent, actually-live
//! `getAncestorDirectoryPaths`, which belongs to that file's own scope, not
//! this one, and is out of this item's scope regardless per the brief).
//! `getExplorerTargetPath` is the file's only export with a real, non-test,
//! non-self-file caller (`use-file-explorer-sync.ts`'s every-render
//! `useMemo`).
//!
//! # `ExplorerTargetBuffer`: a narrow local stand-in for `PaneContent`
//!
//! The TS signature is `getExplorerTargetPath(activeBuffer: PaneContent |
//! null): string | undefined`. `PaneContent`
//! (`web/src/features/panes/types/pane-content.ts`) is a ten-variant
//! discriminated union covering the whole pane surface (editor / terminal /
//! newTab / commitDiff / markdownPreview / htmlPreview / csvPreview /
//! externalEditor / branchReview / agentChat) — Phase-4 pane state, not
//! named anywhere in this item's scope. Porting the whole union here just to
//! support one function's `if` chain would pull an unrelated feature area
//! into `crowbar-core`.
//!
//! [`ExplorerTargetBuffer`] instead represents exactly what
//! [`get_explorer_target_path`] reads: which of five outcomes a buffer is,
//! and one path-shaped field off the three that carry one. Everything else
//! about `PaneContent` (editor content/dirty flags, terminal session ids,
//! agent-chat runner ids, ...) is irrelevant to this function and has no
//! representation here.
//!
//! **Flagged for reconciliation**, the same shape
//! `crate::settings::types::FileTreeDensity`'s module doc already used for a
//! different cross-area dependency: when `PaneContent`/panes is ported to
//! `crowbar-core`, [`ExplorerTargetBuffer`] should be deleted and
//! [`get_explorer_target_path`] repointed at the real union.

/// See this module's doc for why this is a narrow local stand-in for
/// `PaneContent`, not a port of the whole union.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ExplorerTargetBuffer {
    /// TS `type: 'markdownPreview'` — targets `sourceFilePath`.
    MarkdownPreview { source_file_path: String },
    /// TS `type: 'htmlPreview'` — targets `sourceFilePath`.
    HtmlPreview { source_file_path: String },
    /// TS `type: 'csvPreview'` — targets `sourceFilePath`.
    CsvPreview { source_file_path: String },
    /// TS `type: 'editor'` — targets `path`.
    Editor { path: String },
    /// TS `type: 'externalEditor'` — targets `path`.
    ExternalEditor { path: String },
    /// Every other `PaneContentType` (`terminal`, `newTab`, `commitDiff`,
    /// `branchReview`, `agentChat`) — `getExplorerTargetPath` returns
    /// `undefined` for all of them alike, so they collapse to one variant
    /// here instead of five that would each carry fields this function
    /// never reads.
    Other,
}

/// Mirrors `getExplorerTargetPath`. `None` for a missing buffer or for any
/// pane kind that isn't a preview/editor kind.
#[must_use]
pub fn get_explorer_target_path(active_buffer: Option<&ExplorerTargetBuffer>) -> Option<String> {
    match active_buffer? {
        ExplorerTargetBuffer::MarkdownPreview { source_file_path }
        | ExplorerTargetBuffer::HtmlPreview { source_file_path }
        | ExplorerTargetBuffer::CsvPreview { source_file_path } => Some(source_file_path.clone()),
        ExplorerTargetBuffer::Editor { path } | ExplorerTargetBuffer::ExternalEditor { path } => {
            Some(path.clone())
        }
        ExplorerTargetBuffer::Other => None,
    }
}

#[cfg(test)]
mod tests {
    use super::{ExplorerTargetBuffer, get_explorer_target_path};

    // === ported from file-explorer-tree-utils.test.ts's `getExplorerTargetPath` describe ===

    #[test]
    fn uses_the_source_file_for_preview_buffers() {
        let buffer = ExplorerTargetBuffer::MarkdownPreview {
            source_file_path: "/workspace/README.md".to_string(),
        };
        assert_eq!(
            get_explorer_target_path(Some(&buffer)),
            Some("/workspace/README.md".to_string())
        );
    }

    #[test]
    fn ignores_non_file_buffers() {
        let buffer = ExplorerTargetBuffer::Other;
        assert_eq!(get_explorer_target_path(Some(&buffer)), None);
    }

    // --- new: not exercised by the ported TS suite, which only covers one
    //     preview kind and one non-file kind — filling out the rest of the
    //     match so every arm is proven, not just the first two ---

    #[test]
    fn returns_none_for_a_missing_buffer() {
        assert_eq!(get_explorer_target_path(None), None);
    }

    #[test]
    fn targets_path_for_editor_and_external_editor_buffers() {
        assert_eq!(
            get_explorer_target_path(Some(&ExplorerTargetBuffer::Editor {
                path: "/workspace/src/main.rs".to_string(),
            })),
            Some("/workspace/src/main.rs".to_string())
        );
        assert_eq!(
            get_explorer_target_path(Some(&ExplorerTargetBuffer::ExternalEditor {
                path: "/workspace/src/main.rs".to_string(),
            })),
            Some("/workspace/src/main.rs".to_string())
        );
    }

    #[test]
    fn targets_source_file_for_html_and_csv_previews_too() {
        assert_eq!(
            get_explorer_target_path(Some(&ExplorerTargetBuffer::HtmlPreview {
                source_file_path: "/workspace/report.html".to_string(),
            })),
            Some("/workspace/report.html".to_string())
        );
        assert_eq!(
            get_explorer_target_path(Some(&ExplorerTargetBuffer::CsvPreview {
                source_file_path: "/workspace/data.csv".to_string(),
            })),
            Some("/workspace/data.csv".to_string())
        );
    }
}

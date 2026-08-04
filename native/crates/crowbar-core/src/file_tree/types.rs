//! The file-tree record types — ported from `web/src/features/file-system/
//! types/app.ts`.
//!
//! # Why [`AppFile`] carries fewer fields than the TS `AppFile` interface
//!
//! The TS interface has 13 fields; this struct has 7. The other 6
//! (`isDirectory` — a dead alias of `isDir`, verified by grep: nothing in
//! `web/src` reads it, only `isDir` is ever checked — `isFile`, `isSymlink`,
//! `symlinkTarget`, `ignored`, `gitStatus`) are read or written by exactly
//! zero of the five functions this crate's `file_tree` module ports:
//! [`super::visible_rows`]'s tree walk touches `path`/`name`/`isDir`/
//! `isEditing`/`isRenaming`/`isNewItem`/`children` and nothing else;
//! [`super::git_status`]'s `getFileTreeEntryGitStatusDecoration` touches only
//! `path`/`isDir`. `isSymlink`/`symlinkTarget` are read by
//! `file-explorer-tree-item.tsx` (a component) and `ignored` by
//! `file-tree-gitignore.ts` (explicitly out of this item's scope, per
//! `native/mapping/core-filetree.md`) — both live TS fields, just not
//! consumed by anything ported here. Matching `crate::git::types::GitDiff`'s
//! own precedent (see its module doc): a field with no producer and no
//! consumer among the ported functions is not carried, rather than declared
//! and left permanently untested. Whoever next ports the component/store
//! layer that does read `isSymlink`/`symlinkTarget`/`ignored`/`gitStatus`
//! should add them back then, not before.

/// Mirrors `types/app.ts`'s `AppFile` (aliased there as `FileEntry` — see
/// [`FileEntry`]), trimmed to the fields this crate's ported functions
/// actually read or write. See this module's doc for which TS fields were
/// dropped and why.
///
/// `clippy::struct_excessive_bools` (pedantic) flags the four independent UI
/// flags below. Matching `crate::settings::types::Settings`'s own precedent
/// for the same lint: these are four genuinely unrelated toggles the TS
/// source itself models as four separate optional booleans, none of them
/// validated or clamped against each other — inventing a bespoke state
/// machine to satisfy the lint would add indirection the source's own shape
/// does not call for.
#[derive(Debug, Clone, PartialEq, Default)]
#[allow(clippy::struct_excessive_bools)]
pub struct AppFile {
    pub path: String,
    pub name: String,
    /// TS: `isDir?: boolean`, documented as "Alias for isDirectory" — but
    /// `isDirectory` itself is never read anywhere in `web/src`, so this is
    /// the one of the pair that is actually live (see this module's doc).
    pub is_dir: bool,
    /// UI state: this entry is mid-rename.
    pub is_renaming: bool,
    /// UI state: this entry is mid-create/edit (an inline-edit placeholder).
    pub is_editing: bool,
    /// UI state: a newly created item not yet saved. [`super::visible_rows`]
    /// treats this as never "expanded" even if its path happens to collide
    /// with an expanded real directory's — see that module's doc.
    pub is_new_item: bool,
    pub children: Option<Vec<AppFile>>,
}

/// TS: `export type FileEntry = AppFile` — a bare alias, not a second type.
/// Ported the same way: a Rust type alias, not a duplicate struct.
pub type FileEntry = AppFile;

/// `ContextMenuState`'s `position` field (`{ x: number; y: number }` inline
/// in the TS source).
#[derive(Debug, Clone, Copy, PartialEq, Default)]
pub struct ContextMenuPosition {
    pub x: f64,
    pub y: f64,
}

/// Mirrors `types/app.ts`'s `ContextMenuState<T = unknown>`.
///
/// **Not read or constructed by any of this item's five in-scope
/// functions** — the TS file's sole production consumer,
/// `use-file-explorer-context-menu.tsx`, is a CONDITIONAL hook explicitly out
/// of this item's scope (see `native/mapping/core-filetree.md`). Ported
/// anyway because this item's brief names it directly alongside `AppFile`/
/// `FileEntry` as one of "the record types [this item's functions] need" —
/// kept here as a plain data shape with no behaviour of its own to get
/// wrong, rather than silently dropped and re-derived later from a stale TS
/// reading. `T` defaults to `()`, matching TS's `T = unknown` default in the
/// absence of any caller in scope to infer a concrete type from.
#[derive(Debug, Clone, PartialEq, Default)]
pub struct ContextMenuState<T = ()> {
    pub x: Option<f64>,
    pub y: Option<f64>,
    pub is_open: Option<bool>,
    pub position: Option<ContextMenuPosition>,
    pub data: Option<T>,
    pub path: String,
    pub is_dir: bool,
    pub is_file: Option<bool>,
    pub name: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::{AppFile, ContextMenuPosition, ContextMenuState, FileEntry};

    // --- new: no TS test file for this module (`types/app.ts` has none) ---

    #[test]
    fn file_entry_is_the_same_type_as_app_file() {
        let file: FileEntry = AppFile {
            path: "src/main.rs".to_string(),
            name: "main.rs".to_string(),
            is_dir: false,
            is_renaming: false,
            is_editing: false,
            is_new_item: false,
            children: None,
        };
        assert_eq!(file.name, "main.rs");
    }

    #[test]
    fn app_file_default_is_an_unnamed_non_directory_leaf() {
        let file = AppFile::default();
        assert_eq!(file.path, "");
        assert!(!file.is_dir);
        assert!(file.children.is_none());
    }

    #[test]
    fn context_menu_state_holds_an_optional_typed_payload() {
        let state: ContextMenuState<u32> = ContextMenuState {
            data: Some(7),
            path: "src".to_string(),
            is_dir: true,
            position: Some(ContextMenuPosition { x: 10.0, y: 20.0 }),
            ..ContextMenuState::default()
        };
        assert_eq!(state.data, Some(7));
        assert_eq!(
            state.position,
            Some(ContextMenuPosition { x: 10.0, y: 20.0 })
        );
    }
}

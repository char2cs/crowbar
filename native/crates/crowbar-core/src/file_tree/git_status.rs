//! Git-status → per-row file-tree decoration, with directory-level status
//! propagated from the highest-priority child.
//!
//! Ported from `web/src/features/file-explorer/file-explorer/lib/
//! file-tree-git-status.ts` (122 lines, all 6 exports).
//!
//! # The `colorClassName` split, per this item's brief
//!
//! The TS `FileTreeGitStatusDecoration` interface bundles three fields:
//! `colorClassName` (a hardcoded Tailwind string, e.g.
//! `'text-git-modified-staged'`), `label` (`'Modified (staged)'`), and
//! `statusLetter` (`'M'`). The first is presentation; the other two are the
//! actual classification ("which of the five git-status kinds is this,
//! staged or not"). `crowbar-core` must never depend on `gpui` (§4.3 rule 1)
//! and has no business owning a Tailwind class string regardless — that is
//! exactly `crate::color`'s established boundary (see its module doc): this
//! crate does colour *arithmetic*, `crowbar-ui` *paints*.
//!
//! [`FileTreeGitStatusDecoration`] is sealed as a 6-variant enum rather than
//! ported as a struct-of-three-strings: TS's three fields always co-vary
//! (there is no `'modified'`/`'A'`/`'Deleted'` combination the original
//! `switch` can produce), so an enum makes an invalid combination
//! unrepresentable instead of merely untested. [`FileTreeGitStatusDecoration::status_letter`]
//! and [`FileTreeGitStatusDecoration::label`] carry the classification;
//! [`FileTreeGitStatusDecoration::color`] is the seam — it returns a
//! [`GitStatusColor`], a second, wholly separate enum with no string in it
//! anywhere, which `crowbar-ui` will be the one to resolve into an actual
//! themed colour (a `crowbar-ui::Color` seal, per this item's brief) once
//! that crate exists. This *is* the split "at the type level": a caller that
//! only wants the classification never touches [`GitStatusColor`], and nothing
//! in this crate ever converts [`GitStatusColor`] into a paintable value.
//!
//! # Types reused from `crowbar-proto`, matching `crate::git`'s precedent
//!
//! [`crowbar_proto::api_v0_dto::GitFileDTO`] (`path`/`status`/`staged`) and
//! [`crowbar_proto::api_v0_dto::GitStatusDTO`] (`branch`/`ahead`/`behind`/
//! `files`) line up field-for-field with the TS `GitFile`/`GitStatus`
//! interfaces this file imports — reused directly rather than hand-rolled,
//! following `crate::git::types`'s own module doc for the same situation.
//! `GitFileDTO.status` is a plain `String`, not a sealed enum (the wire
//! shape has no closed union), so [`get_file_tree_git_status_decoration`]
//! matches on `&str` and falls back to `None` for anything outside the five
//! known kinds — mirroring the TS `switch`'s own `default: return null`.
//!
//! # A private, narrow port of `getRelativePath`
//!
//! [`get_file_tree_entry_git_status_decoration`] needs `@/utils/
//! path-helpers.ts`'s `getRelativePath` (plus its own three helpers,
//! `normalizePath`/`stripTrailingPathSeparators`/`pathStartsWithRoot`) —
//! but `path-helpers.ts` itself is a general cross-feature utility, not one
//! of the five files this item's scope names. Ported privately below (not
//! `pub`) rather than pulled in as a sixth file — the same "a cross-area
//! dependency this item could not defer" shape
//! `crate::settings::types::FileTreeDensity`'s module doc already documents
//! for a different file, just resolved locally instead of with a permanent
//! type-level fork, since nothing else in this crate needs it yet. If
//! `path-helpers.ts` is ever ported to `crowbar-core` directly, delete this
//! copy and repoint at that module's version.

use std::collections::HashMap;

use crowbar_proto::api_v0_dto::{GitFileDTO, GitStatusDTO};

use super::types::FileEntry;

/// The status-letter/label classification half of TS's
/// `FileTreeGitStatusDecoration` — see this module's doc for why
/// `colorClassName` is not a field here.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FileTreeGitStatusDecoration {
    Modified,
    ModifiedStaged,
    Added,
    Deleted,
    Untracked,
    Renamed,
}

impl FileTreeGitStatusDecoration {
    /// The single trailing letter shown after the filename (M / A / U / D /
    /// R). `Modified` and `ModifiedStaged` share `'M'` — staged-ness changes
    /// the label, not the letter, matching the TS source exactly.
    #[must_use]
    pub fn status_letter(self) -> char {
        match self {
            Self::Modified | Self::ModifiedStaged => 'M',
            Self::Added => 'A',
            Self::Deleted => 'D',
            Self::Untracked => 'U',
            Self::Renamed => 'R',
        }
    }

    #[must_use]
    pub fn label(self) -> &'static str {
        match self {
            Self::Modified => "Modified",
            Self::ModifiedStaged => "Modified (staged)",
            Self::Added => "Added",
            Self::Deleted => "Deleted",
            Self::Untracked => "Untracked",
            Self::Renamed => "Renamed",
        }
    }

    /// The colour seal this classification resolves to. See this module's
    /// doc for why this is a distinct type rather than a string field.
    #[must_use]
    pub fn color(self) -> GitStatusColor {
        match self {
            Self::Modified => GitStatusColor::Modified,
            Self::ModifiedStaged => GitStatusColor::ModifiedStaged,
            Self::Added => GitStatusColor::Added,
            Self::Deleted => GitStatusColor::Deleted,
            Self::Untracked => GitStatusColor::Untracked,
            Self::Renamed => GitStatusColor::Renamed,
        }
    }
}

/// The colour half of TS's `FileTreeGitStatusDecoration.colorClassName`
/// (`'text-git-modified-staged'`, `'text-git-added'`, ...) — a hardcoded
/// Tailwind class string in the TS source, replaced here by a plain enum
/// with no colour value baked in. `crowbar-ui` is the intended resolver: it
/// will match on this and look up its own themed colour (a
/// `crowbar-ui::Color` seal, once that crate exists), instead of parsing or
/// trusting a loose class-name string the way the TS source does.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum GitStatusColor {
    Modified,
    ModifiedStaged,
    Added,
    Deleted,
    Untracked,
    Renamed,
}

/// Mirrors `FileTreeGitStatusLookup`.
#[derive(Debug, Clone, Default)]
pub struct FileTreeGitStatusLookup {
    pub files: HashMap<String, FileTreeGitStatusDecoration>,
    pub directories: HashMap<String, FileTreeGitStatusDecoration>,
}

/// Mirrors `gitStatusPriority` (module-private `Record` in the TS source).
/// The `_ => 0` arm is defensive, matching the TS source's own
/// `gitStatusPriority[gitFile.status] ?? 0`: a `Record<GitFile['status'],
/// number>` indexed by TS's closed 5-member union can never actually miss,
/// same as this fallback is never reached via
/// [`get_git_status_priority`]'s one real caller
/// ([`create_file_tree_git_status_lookup`] already filters to the five
/// known statuses via [`get_file_tree_git_status_decoration`] before ever
/// calling in). Exercised directly below rather than left as an
/// unreachable-in-practice gap with no test at all.
fn git_status_priority(status: &str) -> i32 {
    match status {
        "deleted" => 50,
        "modified" => 40,
        "renamed" => 30,
        "added" => 20,
        "untracked" => 10,
        _ => 0,
    }
}

/// Mirrors `getGitStatusPriority` (module-private in the TS source too): a
/// staged modification outranks an unstaged one by exactly one point, so it
/// always wins a tie against another unstaged `modified` entry without ever
/// crossing into the next priority tier (`renamed`, 30).
fn get_git_status_priority(git_file: &GitFileDTO) -> i32 {
    let priority = git_status_priority(&git_file.status);
    if git_file.status == "modified" && git_file.staged {
        priority + 1
    } else {
        priority
    }
}

/// Mirrors `getFileTreeGitStatusDecoration`. `None` for any status string
/// outside the five known kinds, matching the TS `switch`'s `default: return
/// null`.
#[must_use]
pub fn get_file_tree_git_status_decoration(
    git_file: &GitFileDTO,
) -> Option<FileTreeGitStatusDecoration> {
    match git_file.status.as_str() {
        "modified" => Some(if git_file.staged {
            FileTreeGitStatusDecoration::ModifiedStaged
        } else {
            FileTreeGitStatusDecoration::Modified
        }),
        "added" => Some(FileTreeGitStatusDecoration::Added),
        "deleted" => Some(FileTreeGitStatusDecoration::Deleted),
        "untracked" => Some(FileTreeGitStatusDecoration::Untracked),
        "renamed" => Some(FileTreeGitStatusDecoration::Renamed),
        _ => None,
    }
}

/// Mirrors `createFileTreeGitStatusLookup`: builds both the exact-file
/// lookup and the directory lookup, where each ancestor directory inherits
/// the highest-priority status among its (possibly many) changed
/// descendants.
#[must_use]
pub fn create_file_tree_git_status_lookup(git_status: &GitStatusDTO) -> FileTreeGitStatusLookup {
    let mut files = HashMap::new();
    let mut directories = HashMap::new();
    let mut directory_priorities: HashMap<String, i32> = HashMap::new();

    for git_file in &git_status.files {
        let Some(status_decoration) = get_file_tree_git_status_decoration(git_file) else {
            continue;
        };

        files.insert(git_file.path.clone(), status_decoration);

        let segments: Vec<&str> = git_file.path.split('/').collect();
        let mut current_path = String::new();
        let ancestor_segment_count = segments.len().saturating_sub(1);
        for segment in &segments[..ancestor_segment_count] {
            current_path = if current_path.is_empty() {
                (*segment).to_string()
            } else {
                format!("{current_path}/{segment}")
            };
            let next_priority = get_git_status_priority(git_file);
            let current_priority = directory_priorities
                .get(&current_path)
                .copied()
                .unwrap_or(-1);
            if next_priority > current_priority {
                directories.insert(current_path.clone(), status_decoration);
                directory_priorities.insert(current_path.clone(), next_priority);
            }
        }
    }

    FileTreeGitStatusLookup { files, directories }
}

/// Mirrors `resolveActiveWorkspaceGitStatus`.
///
/// Decides whether `workspace_git_status` applies to the file tree currently
/// on screen. The file explorer always renders the ACTIVE workspace; the
/// caller's git-status store keys its cached status by the workspace id it
/// loaded (`current_workspace_repo_path`). Applying status from a stale
/// workspace would briefly paint the previous workspace's decorations onto
/// the new tree during a workspace switch — this returns `None` instead in
/// that window. An empty-string id is treated the same as a missing one
/// (matching TS's `!currentWorkspaceRepoPath`/`!activeWorkspaceId`
/// falsiness checks, not just their `null` checks — not exercised by the TS
/// suite, which only tests `null`, but a faithful port of the actual guard).
#[must_use]
pub fn resolve_active_workspace_git_status<'a>(
    workspace_git_status: Option<&'a GitStatusDTO>,
    current_workspace_repo_path: Option<&str>,
    active_workspace_id: Option<&str>,
) -> Option<&'a GitStatusDTO> {
    let workspace_git_status = workspace_git_status?;
    let current = current_workspace_repo_path.filter(|s| !s.is_empty())?;
    let active = active_workspace_id.filter(|s| !s.is_empty())?;
    (current == active).then_some(workspace_git_status)
}

/// Mirrors `getFileTreeEntryGitStatusDecoration`. `None` without a (non-empty)
/// root path or lookup, or when `file`'s path has no entry (or inherited
/// directory entry) in `lookup`.
#[must_use]
pub fn get_file_tree_entry_git_status_decoration(
    file: &FileEntry,
    root_folder_path: Option<&str>,
    lookup: Option<&FileTreeGitStatusLookup>,
) -> Option<FileTreeGitStatusDecoration> {
    let root_folder_path = root_folder_path.filter(|s| !s.is_empty())?;
    let lookup = lookup?;

    let relative_path = get_relative_path(&file.path, root_folder_path);
    if relative_path.is_empty() {
        return None;
    }

    if let Some(status) = lookup.files.get(&relative_path) {
        return Some(*status);
    }

    if file.is_dir {
        return lookup.directories.get(&relative_path).copied();
    }

    None
}

// --- private port of `@/utils/path-helpers.ts` — see this module's doc ---

fn normalize_path(path: &str) -> String {
    path.replace('\\', "/")
}

fn is_drive_root(path: &str) -> bool {
    // TS: /^[A-Za-z]:[\\/]$/
    let bytes = path.as_bytes();
    bytes.len() == 3
        && bytes[0].is_ascii_alphabetic()
        && bytes[1] == b':'
        && matches!(bytes[2], b'\\' | b'/')
}

fn strip_trailing_path_separators(path: &str) -> String {
    if path == "/" || path == "\\" || is_drive_root(path) {
        return path.to_string();
    }
    path.trim_end_matches(['/', '\\']).to_string()
}

fn starts_with_drive_letter(path: &str) -> bool {
    // TS: /^[A-Za-z]:\//
    let bytes = path.as_bytes();
    bytes.len() >= 3 && bytes[0].is_ascii_alphabetic() && bytes[1] == b':' && bytes[2] == b'/'
}

fn path_starts_with_root(full_path: &str, root_path: &str) -> bool {
    let normalized_full_path = normalize_path(&strip_trailing_path_separators(full_path));
    let normalized_root_path = normalize_path(&strip_trailing_path_separators(root_path));

    let full_for_compare = if starts_with_drive_letter(&normalized_full_path) {
        normalized_full_path.to_lowercase()
    } else {
        normalized_full_path
    };
    let root_for_compare = if starts_with_drive_letter(&normalized_root_path) {
        normalized_root_path.to_lowercase()
    } else {
        normalized_root_path
    };

    let root_prefix = if root_for_compare.ends_with('/') {
        root_for_compare.clone()
    } else {
        format!("{root_for_compare}/")
    };

    full_for_compare == root_for_compare || full_for_compare.starts_with(&root_prefix)
}

/// Mirrors `getRelativePath(fullPath, rootFolderPath)`, with
/// `rootFolderPath` required non-empty (callers of this private helper
/// already guard the empty/missing case — see
/// [`get_file_tree_entry_git_status_decoration`]).
fn get_relative_path(full_path: &str, root_folder_path: &str) -> String {
    let normalized_full_path = normalize_path(&strip_trailing_path_separators(full_path));
    let normalized_root_path = normalize_path(&strip_trailing_path_separators(root_folder_path));

    if path_starts_with_root(full_path, root_folder_path) {
        if normalized_full_path.len() == normalized_root_path.len() {
            return String::new();
        }
        let relative_offset = if normalized_root_path.ends_with('/') {
            normalized_root_path.len()
        } else {
            normalized_root_path.len() + 1
        };
        return normalized_full_path
            .get(relative_offset..)
            .map(str::to_string)
            .unwrap_or_default();
    }

    full_path.to_string()
}

#[cfg(test)]
mod tests {
    use super::{
        FileTreeGitStatusDecoration, create_file_tree_git_status_lookup,
        get_file_tree_entry_git_status_decoration, get_file_tree_git_status_decoration,
        resolve_active_workspace_git_status,
    };
    use crate::file_tree::types::AppFile;
    use crowbar_proto::api_v0_dto::{GitFileDTO, GitStatusDTO};

    fn git_file(path: &str, status: &str, staged: bool) -> GitFileDTO {
        GitFileDTO {
            path: path.to_string(),
            status: status.to_string(),
            staged,
        }
    }

    fn file_entry(path: &str, is_dir: bool) -> AppFile {
        AppFile {
            path: path.to_string(),
            name: path.rsplit('/').next().unwrap_or(path).to_string(),
            is_dir,
            ..AppFile::default()
        }
    }

    // === getFileTreeGitStatusDecoration — ported from file-tree-git-status.test.ts ===

    #[test]
    fn maps_modified_files_to_staged_and_unstaged_variants() {
        assert_eq!(
            get_file_tree_git_status_decoration(&git_file("src/app.ts", "modified", false)),
            Some(FileTreeGitStatusDecoration::Modified)
        );
        assert_eq!(
            get_file_tree_git_status_decoration(&git_file("src/app.ts", "modified", true)),
            Some(FileTreeGitStatusDecoration::ModifiedStaged)
        );
    }

    #[test]
    fn maps_non_modified_statuses_to_their_file_tree_variants() {
        assert_eq!(
            get_file_tree_git_status_decoration(&git_file("added.ts", "added", false)),
            Some(FileTreeGitStatusDecoration::Added)
        );
        assert_eq!(
            get_file_tree_git_status_decoration(&git_file("deleted.ts", "deleted", false)),
            Some(FileTreeGitStatusDecoration::Deleted)
        );
        assert_eq!(
            get_file_tree_git_status_decoration(&git_file("untracked.ts", "untracked", false)),
            Some(FileTreeGitStatusDecoration::Untracked)
        );
        assert_eq!(
            get_file_tree_git_status_decoration(&git_file("renamed.ts", "renamed", false)),
            Some(FileTreeGitStatusDecoration::Renamed)
        );
    }

    #[test]
    fn status_letter_and_label_match_the_ts_table() {
        assert_eq!(FileTreeGitStatusDecoration::Modified.status_letter(), 'M');
        assert_eq!(FileTreeGitStatusDecoration::Modified.label(), "Modified");
        assert_eq!(
            FileTreeGitStatusDecoration::ModifiedStaged.status_letter(),
            'M'
        );
        assert_eq!(
            FileTreeGitStatusDecoration::ModifiedStaged.label(),
            "Modified (staged)"
        );
        assert_eq!(FileTreeGitStatusDecoration::Added.status_letter(), 'A');
        assert_eq!(FileTreeGitStatusDecoration::Deleted.status_letter(), 'D');
        assert_eq!(FileTreeGitStatusDecoration::Untracked.status_letter(), 'U');
        assert_eq!(FileTreeGitStatusDecoration::Renamed.status_letter(), 'R');
        assert_eq!(FileTreeGitStatusDecoration::Added.label(), "Added");
        assert_eq!(FileTreeGitStatusDecoration::Deleted.label(), "Deleted");
        assert_eq!(FileTreeGitStatusDecoration::Untracked.label(), "Untracked");
        assert_eq!(FileTreeGitStatusDecoration::Renamed.label(), "Renamed");
    }

    // --- new: not part of the TS interface at all — proves the split ---

    #[test]
    fn every_classification_resolves_to_a_distinct_color_seal() {
        use super::GitStatusColor;
        assert_eq!(
            FileTreeGitStatusDecoration::Modified.color(),
            GitStatusColor::Modified
        );
        assert_eq!(
            FileTreeGitStatusDecoration::ModifiedStaged.color(),
            GitStatusColor::ModifiedStaged
        );
        assert_eq!(
            FileTreeGitStatusDecoration::Added.color(),
            GitStatusColor::Added
        );
        assert_eq!(
            FileTreeGitStatusDecoration::Deleted.color(),
            GitStatusColor::Deleted
        );
        assert_eq!(
            FileTreeGitStatusDecoration::Untracked.color(),
            GitStatusColor::Untracked
        );
        assert_eq!(
            FileTreeGitStatusDecoration::Renamed.color(),
            GitStatusColor::Renamed
        );
    }

    #[test]
    fn an_unknown_status_string_decorates_to_none() {
        assert_eq!(
            get_file_tree_git_status_decoration(&git_file("x.ts", "conflicted", false)),
            None
        );
    }

    #[test]
    fn git_status_priority_falls_back_to_zero_for_an_unknown_status() {
        // Direct test of the private helper's defensive fallback — see its
        // doc comment for why this is unreachable through the public API.
        assert_eq!(super::git_status_priority("conflicted"), 0);
    }

    #[test]
    fn an_unrecognised_status_is_excluded_from_the_lookup_entirely() {
        // create_file_tree_git_status_lookup's `continue` guard: a file whose
        // status doesn't decorate must not appear in either map, not even
        // with a None-ish placeholder.
        let git_status = GitStatusDTO {
            branch: "main".to_string(),
            ahead: 0,
            behind: 0,
            files: vec![git_file("src/weird.ts", "conflicted", false)],
        };
        let lookup = create_file_tree_git_status_lookup(&git_status);
        assert!(lookup.files.is_empty());
        assert!(lookup.directories.is_empty());
    }

    #[test]
    fn a_staged_modification_outranks_an_unstaged_one_for_directory_propagation() {
        // get_git_status_priority's +1 staged bonus (this module's own doc):
        // both files are 'modified' (same base priority, 40), so without the
        // bonus this would be a tie the map insertion order would decide
        // arbitrarily. With it, the staged entry always wins regardless of
        // which one is listed first.
        let staged_first = create_file_tree_git_status_lookup(&GitStatusDTO {
            branch: "main".to_string(),
            ahead: 0,
            behind: 0,
            files: vec![
                git_file("src/staged.ts", "modified", true),
                git_file("src/unstaged.ts", "modified", false),
            ],
        });
        assert_eq!(
            staged_first.directories.get("src").copied(),
            Some(FileTreeGitStatusDecoration::ModifiedStaged)
        );

        let unstaged_first = create_file_tree_git_status_lookup(&GitStatusDTO {
            branch: "main".to_string(),
            ahead: 0,
            behind: 0,
            files: vec![
                git_file("src/unstaged.ts", "modified", false),
                git_file("src/staged.ts", "modified", true),
            ],
        });
        assert_eq!(
            unstaged_first.directories.get("src").copied(),
            Some(FileTreeGitStatusDecoration::ModifiedStaged)
        );
    }

    // === private path-helpers port — Windows-style paths, not exercised by
    //     any of this item's ported TS test suites (path-helpers.ts itself
    //     is out of this item's scope; these prove the private port handles
    //     the shapes its own source branches on, read directly from
    //     @/utils/path-helpers.ts rather than from a test file) ===

    #[test]
    fn get_relative_path_strips_a_drive_letter_root_case_insensitively() {
        assert_eq!(
            super::get_relative_path(r"C:\workspace\src\main.rs", r"c:\workspace"),
            "src/main.rs"
        );
    }

    #[test]
    fn get_relative_path_treats_the_filesystem_root_as_a_prefix_ending_in_slash() {
        // root_folder_path == "/" is preserved by strip_trailing_path_separators
        // (unlike any other trailing slash, which is stripped), so
        // path_starts_with_root's root_prefix is already "/" without needing
        // to append one — the `ends_with('/')` branch this exercises.
        assert_eq!(super::get_relative_path("/etc/hosts", "/"), "etc/hosts");
    }

    #[test]
    fn get_relative_path_falls_back_to_the_full_path_outside_the_root() {
        assert_eq!(
            super::get_relative_path("/other/place", "/workspace"),
            "/other/place"
        );
    }

    #[test]
    fn strip_trailing_path_separators_preserves_a_bare_backslash_and_a_drive_root() {
        assert_eq!(super::strip_trailing_path_separators("\\"), "\\");
        assert_eq!(super::strip_trailing_path_separators(r"C:\"), r"C:\");
        // An ordinary path still has its trailing separators stripped.
        assert_eq!(
            super::strip_trailing_path_separators("/workspace/"),
            "/workspace"
        );
    }

    // === file tree git status lookup — ported from file-tree-git-status.test.ts ===

    #[test]
    fn keeps_exact_file_status_and_inherited_directory_status_separate() {
        let git_status = GitStatusDTO {
            branch: "main".to_string(),
            ahead: 0,
            behind: 0,
            files: vec![
                git_file("src/app.ts", "modified", false),
                git_file("docs/readme.md", "added", false),
            ],
        };
        let lookup = create_file_tree_git_status_lookup(&git_status);

        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("/workspace/src/app.ts", false),
                Some("/workspace"),
                Some(&lookup),
            ),
            Some(FileTreeGitStatusDecoration::Modified)
        );
        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("/workspace/src", true),
                Some("/workspace"),
                Some(&lookup),
            ),
            Some(FileTreeGitStatusDecoration::Modified)
        );
        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("/workspace/docs", true),
                Some("/workspace"),
                Some(&lookup),
            ),
            Some(FileTreeGitStatusDecoration::Added)
        );
    }

    #[test]
    fn uses_the_highest_priority_descendant_status_for_directories() {
        let git_status = GitStatusDTO {
            branch: "main".to_string(),
            ahead: 0,
            behind: 0,
            files: vec![
                git_file("src/new.ts", "untracked", false),
                git_file("src/renamed.ts", "renamed", false),
                git_file("src/deleted.ts", "deleted", false),
                git_file("src/modified.ts", "modified", false),
            ],
        };
        let lookup = create_file_tree_git_status_lookup(&git_status);

        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("/workspace/src", true),
                Some("/workspace"),
                Some(&lookup),
            ),
            Some(FileTreeGitStatusDecoration::Deleted)
        );
    }

    #[test]
    fn returns_none_without_a_root_path_or_matching_status() {
        let git_status = GitStatusDTO {
            branch: "main".to_string(),
            ahead: 0,
            behind: 0,
            files: vec![git_file("src/app.ts", "modified", false)],
        };
        let lookup = create_file_tree_git_status_lookup(&git_status);

        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("/workspace/src/app.ts", false),
                None,
                Some(&lookup),
            ),
            None
        );
        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("/workspace/src/other.ts", false),
                Some("/workspace"),
                Some(&lookup),
            ),
            None
        );
    }

    // Regression: the real app addresses file-tree entries by
    // WORKSPACE-RELATIVE paths ("README.md", "api/x.go") while
    // rootFolderPath is the synthetic `/repos/<repoId>` mock-era prefix — a
    // different id space. The git status the backend returns is keyed by the
    // same workspace-relative paths. These resolve correctly even though the
    // file path never starts with rootFolderPath (getRelativePath returns
    // the path unchanged when there is no prefix match).
    #[test]
    fn resolves_decorations_for_workspace_relative_paths_under_a_synthetic_repos_root() {
        let root = "/repos/81883222-d45f-44ca-80ed-9550d5228441";
        let git_status = GitStatusDTO {
            branch: "epoch/first-pr".to_string(),
            ahead: 0,
            behind: 1,
            files: vec![
                git_file("README.md", "modified", false),
                git_file("api/internal/api/container.go", "modified", false),
            ],
        };
        let lookup = create_file_tree_git_status_lookup(&git_status);

        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("README.md", false),
                Some(root),
                Some(&lookup),
            ),
            Some(FileTreeGitStatusDecoration::Modified)
        );
        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("api/internal/api/container.go", false),
                Some(root),
                Some(&lookup),
            ),
            Some(FileTreeGitStatusDecoration::Modified)
        );
        // The parent folder inherits the descendant's status (folder tinting).
        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("api", true),
                Some(root),
                Some(&lookup),
            ),
            Some(FileTreeGitStatusDecoration::Modified)
        );
    }

    // === resolveActiveWorkspaceGitStatus — ported from file-tree-git-status.test.ts ===

    #[test]
    fn applies_the_status_when_the_git_store_loaded_the_active_workspace() {
        let status = GitStatusDTO {
            branch: "epoch/first-pr".to_string(),
            ahead: 0,
            behind: 1,
            files: vec![git_file("README.md", "modified", false)],
        };
        let ws_id = "d2e0a0de-dbee-4fc3-a333-2cac9b6aeff3";
        assert_eq!(
            resolve_active_workspace_git_status(Some(&status), Some(ws_id), Some(ws_id)),
            Some(&status)
        );
    }

    #[test]
    fn rejects_a_status_loaded_for_a_different_workspace() {
        let status = GitStatusDTO {
            branch: "epoch/first-pr".to_string(),
            ahead: 0,
            behind: 1,
            files: vec![git_file("README.md", "modified", false)],
        };
        let ws_id = "d2e0a0de-dbee-4fc3-a333-2cac9b6aeff3";
        assert_eq!(
            resolve_active_workspace_git_status(Some(&status), Some("other-ws-id"), Some(ws_id)),
            None
        );
    }

    #[test]
    fn returns_none_when_any_input_is_missing() {
        let status = GitStatusDTO {
            branch: "epoch/first-pr".to_string(),
            ahead: 0,
            behind: 1,
            files: vec![git_file("README.md", "modified", false)],
        };
        let ws_id = "d2e0a0de-dbee-4fc3-a333-2cac9b6aeff3";
        assert_eq!(
            resolve_active_workspace_git_status(None, Some(ws_id), Some(ws_id)),
            None
        );
        assert_eq!(
            resolve_active_workspace_git_status(Some(&status), None, Some(ws_id)),
            None
        );
        assert_eq!(
            resolve_active_workspace_git_status(Some(&status), Some(ws_id), None),
            None
        );
    }

    // --- new: not exercised by the TS suite (which only tests `null`, not
    //     `''`) — proves the falsiness check, not just the null check ---

    #[test]
    fn an_empty_string_workspace_id_is_treated_as_missing_too() {
        let status = GitStatusDTO {
            branch: "main".to_string(),
            ahead: 0,
            behind: 0,
            files: vec![],
        };
        assert_eq!(
            resolve_active_workspace_git_status(Some(&status), Some(""), Some("ws")),
            None
        );
    }

    #[test]
    fn get_file_tree_entry_git_status_decoration_is_none_for_an_exact_root_match() {
        // getRelativePath('/workspace', '/workspace') == '' — the TS source's
        // `if (!relativePath) return null` guard, exercised directly (the
        // lookup is irrelevant here: the guard fires before it is consulted).
        let git_status = GitStatusDTO {
            branch: "main".to_string(),
            ahead: 0,
            behind: 0,
            files: vec![],
        };
        let lookup = create_file_tree_git_status_lookup(&git_status);
        assert_eq!(
            get_file_tree_entry_git_status_decoration(
                &file_entry("/workspace", true),
                Some("/workspace"),
                Some(&lookup),
            ),
            None
        );
    }
}

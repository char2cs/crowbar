//! Cascading `.gitignore` rule resolution across nested directories.
//!
//! Ported from
//! `web/src/features/file-explorer/file-explorer/lib/file-tree-gitignore.ts`
//! (237 lines). All 7 exports are ported — every one is confirmed LIVE at
//! export level (`native/mapping/tier-a-denominator.md` §5's per-export
//! table): [`is_workspace_relative_path`], [`GitIgnoreFileReference`],
//! [`GitIgnoreFileContent`], [`FileTreeGitIgnoreRules`],
//! [`collect_git_ignore_file_references`],
//! [`create_file_tree_git_ignore_rules`], and
//! [`is_path_git_ignored_by_file_tree_rules`].
//!
//! # The dependency decision: the `ignore` crate, verified
//!
//! The TS source uses the npm `ignore@5.3.2` package for two things: (a)
//! `.gitignore`-syntax pattern parsing (`**` globs, leading-`/` anchoring,
//! trailing-`/` directory-only matches, `#` comments, `\`-escapes, and
//! `!`-negation) via `matcher.test(path)` returning `{ignored, unignored}`;
//! and (b) one matcher **per directory**, assembled into a `ruleSets` array
//! by `createFileTreeGitIgnoreRules`.
//!
//! This port uses [`ignore::gitignore::Gitignore`]/[`ignore::gitignore::GitignoreBuilder`]
//! (ripgrep's own crate — already present in this workspace's `Cargo.lock`
//! transitively via `gpui-component -> rust-i18n -> globwalk -> ignore
//! 0.4.31`, so adding it as a direct `crowbar-core` dependency fetches
//! nothing new) — one [`Gitignore`] instance per directory, mirroring the TS
//! file's own `ruleSets` shape exactly. This is **not** a `gpui` dependency
//! (`scripts/check-invariants.sh` rule 1 only forbids `gpui`, checked
//! directly rather than assumed), so it does not touch the crate's one hard
//! constraint.
//!
//! **Verified, not assumed:** [`ignore::Match`] is `{None, Ignore(T),
//! Whitelist(T)}`, and `Gitignore::matched`'s own doc comment states it
//! returns "the highest precedent glob" — i.e. the *last* matching glob in
//! file order wins, which is exactly gitignore's (and npm `ignore`'s) rule
//! that a later pattern overrides an earlier one within the same file. Read
//! directly from the vendored crate source
//! (`~/.cargo/registry/.../ignore-0.4.31/src/gitignore.rs`) rather than
//! taken on the brief's word: its own test `ig7` (`"!src/main.rs\n*.rs"` on
//! `"src/main.rs"` → ignored, because the un-negated pattern comes *after*
//! the negation) and `ignot6` (the same two lines reversed → not ignored)
//! are the identical precedence rule the npm package documents for
//! `unignored`. So the mapping is a direct structural match:
//!
//! | npm `ignore` `test()` | `ignore` crate `matched()` |
//! |---|---|
//! | `{ignored: true, unignored: false}` | `Match::Ignore(_)` |
//! | `{ignored: false, unignored: true}` | `Match::Whitelist(_)` |
//! | `{ignored: false, unignored: false}` | `Match::None` |
//!
//! [`is_path_ignored_by_own_rules`] below folds a `Match` into the same
//! `ignored`-accumulator loop the TS source uses for `result.ignored`/
//! `result.unignored`, so the port is a direct translation, not a
//! reimplementation of the precedence rule.
//!
//! **One real divergence, found by reading the crate source rather than
//! assuming API parity, and resolved rather than papered over:** the TS
//! `toMatcherPath` communicates "this candidate is a directory" by
//! *appending a trailing `/` to the string* it hands to `matcher.test()`,
//! because npm `ignore`'s `test(path)` takes exactly one string argument.
//! `Gitignore::matched(path, is_dir)` instead takes an explicit `is_dir:
//! bool` as a *second* argument and does not expect a trailing separator in
//! `path` — the crate's own test suite matches bare paths (`"foo"`, not
//! `"foo/"`) against directory-only patterns, signalling directory-ness only
//! through the boolean (see `ig8`/`ig27`/`ig30` in the crate's own tests).
//! Appending a trailing slash to the candidate string *and* passing
//! `is_dir: true` would double-signal in a way the crate's glob compiler
//! does not expect (a compiled pattern's trailing `/` is stripped before
//! matching — see `GitignoreBuilder::add_line` — so a candidate that still
//! has one risks a spurious segment mismatch). [`to_matcher_path`] below
//! therefore does **not** port the trailing-slash append; directory-ness is
//! carried solely through the `is_dir` parameter threaded to
//! [`ignore::gitignore::Gitignore::matched`]. This is verified by this
//! module's own
//! `tests::handles_anchored_nested_patterns_and_directory_only_patterns`
//! (ported directly from the TS suite's identically-named case), not merely
//! reasoned about.
//!
//! What the crate does **not** provide, and this file's own original
//! contribution, is the **ancestor-first cascade**:
//! [`is_path_git_ignored_by_file_tree_rules`] walks every ancestor directory
//! (via the local, live [`get_ancestor_directory_paths`] — see its own doc
//! comment for why it is the local copy and not the dead exported twin) and
//! tests each ancestor's own accumulated rules *before* testing the target
//! path, because a directory ignored by a parent rule ignores everything
//! beneath it regardless of that subdirectory's own `.gitignore`. That
//! algorithm has no crate equivalent (`ignore::WalkBuilder`/`Ignore` do
//! something related during directory *traversal*, but this file never
//! walks the filesystem — it resolves rules against an already-loaded,
//! in-memory tree) and is reimplemented here exactly as the TS source
//! structures it, not merged into some crate-provided combinator.

use std::collections::HashMap;

use ignore::Match;
use ignore::gitignore::{Gitignore, GitignoreBuilder};

const GITIGNORE_FILE_NAME: &str = ".gitignore";

// --- path-helper subset -----------------------------------------------
//
// The TS source imports `getDirName`/`getRelativePath`/`normalizePath`/
// `pathStartsWithRoot`/`stripTrailingPathSeparators` from the separate
// `web/src/utils/path-helpers.ts`. Porting that whole module is out of this
// item's scope (`file-tree-gitignore.ts`'s own 7 exports only, per the
// brief) — these are private, narrowed re-implementations of exactly the
// five functions this file calls, kept local rather than exported, so a
// future `crowbar-core::path_helpers` port is free to define the full
// module's shape without this file having pre-empted a name for it.

fn normalize_path(path: &str) -> String {
    path.replace('\\', "/")
}

/// A `X:\` / `X:/` Windows drive-letter prefix, checked byte-wise. Matches
/// `path-helpers.ts`'s repeated `/^[A-Za-z]:[\\/]/`-shaped regexes; byte
/// indexing here is safe because `:`/`\`/`/` are single-byte ASCII and can
/// never appear as a UTF-8 continuation byte, so slicing on them can't land
/// mid-character.
fn has_drive_prefix(path: &str) -> bool {
    let bytes = path.as_bytes();
    bytes.len() >= 3
        && bytes[0].is_ascii_alphabetic()
        && bytes[1] == b':'
        && (bytes[2] == b'\\' || bytes[2] == b'/')
}

/// Mirrors `stripTrailingPathSeparators`.
fn strip_trailing_path_separators(path: &str) -> String {
    if path == "/" || path == "\\" {
        return path.to_string();
    }
    if has_drive_prefix(path) && path.len() == 3 {
        return path.to_string();
    }
    path.trim_end_matches(['/', '\\']).to_string()
}

/// Mirrors `getDirName`.
fn get_dir_name(path: &str) -> String {
    let stripped_path = strip_trailing_path_separators(path);
    let last_separator_index = [stripped_path.rfind('/'), stripped_path.rfind('\\')]
        .into_iter()
        .flatten()
        .max();

    let Some(last_separator_index) = last_separator_index else {
        return String::new();
    };

    if has_drive_prefix(&stripped_path) && last_separator_index == 2 {
        return stripped_path[..3].to_string();
    }
    if last_separator_index == 0 {
        // TS falls back to `'/'` if `strippedPath[0]` is falsy, but that
        // index is only ever reached when `strippedPath` has a separator at
        // position 0 — i.e. `strippedPath[0]` is always `'/'` or `'\\'`,
        // never empty. The fallback is unreachable; this mirrors the
        // reachable behaviour exactly.
        return stripped_path[..1].to_string();
    }

    stripped_path[..last_separator_index].to_string()
}

/// Mirrors `pathStartsWithRoot`.
fn path_starts_with_root(full_path: &str, root_path: &str) -> bool {
    let normalized_full_path = normalize_path(&strip_trailing_path_separators(full_path));
    let normalized_root_path = normalize_path(&strip_trailing_path_separators(root_path));

    // Post-normalize, a drive prefix is always followed by `/` (backslashes
    // were already converted), matching `/^[A-Za-z]:\//` in the TS source.
    let drive_prefixed = |p: &str| {
        let b = p.as_bytes();
        b.len() >= 3 && b[0].is_ascii_alphabetic() && b[1] == b':' && b[2] == b'/'
    };

    let full_path_for_compare = if drive_prefixed(&normalized_full_path) {
        normalized_full_path.to_lowercase()
    } else {
        normalized_full_path
    };
    let root_path_for_compare = if drive_prefixed(&normalized_root_path) {
        normalized_root_path.to_lowercase()
    } else {
        normalized_root_path
    };

    let root_prefix = if root_path_for_compare.ends_with('/') {
        root_path_for_compare.clone()
    } else {
        format!("{root_path_for_compare}/")
    };

    full_path_for_compare == root_path_for_compare
        || full_path_for_compare.starts_with(&root_prefix)
}

/// Mirrors `getRelativePath`. The TS signature accepts `string | null |
/// undefined` for `rootFolderPath`; every call site in this file always
/// passes a concrete, already-validated non-empty string, so this narrows to
/// `&str` (the empty-string guard is kept for parity with the `!rootFolderPath`
/// falsy check, since `Option::filter`-style callers upstream already treat
/// `""` as "no root").
fn get_relative_path(full_path: &str, root_folder_path: &str) -> String {
    if root_folder_path.is_empty() {
        return full_path.to_string();
    }

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
        return normalized_full_path[relative_offset..].to_string();
    }

    full_path.to_string()
}

// --- the file's own tree-entry shape -----------------------------------

/// A narrow, gitignore-scoped mirror of
/// `web/src/features/file-system/types/app.ts`'s `FileEntry` (= `AppFile`),
/// carrying only the fields [`collect_git_ignore_file_references`] actually
/// reads: `name`, `path`, `isDir`, `children`. Porting the full `AppFile`/
/// `FileEntry` type is out of this item's scope (this file's own 7 exports
/// only); named distinctly (not `FileEntry`) so a fuller port elsewhere in
/// `crowbar-core::filetree` does not collide with this name.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct GitIgnoreTreeEntry {
    pub name: String,
    pub path: String,
    pub is_dir: bool,
    pub children: Vec<GitIgnoreTreeEntry>,
}

// --- exported types ------------------------------------------------------

/// Mirrors the exported `GitIgnoreFileReference` interface.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct GitIgnoreFileReference {
    pub path: String,
    pub directory_path: String,
}

/// Mirrors the exported `GitIgnoreFileContent` interface. TS declares this as
/// `extends GitIgnoreFileReference`; every call site in the source and its
/// test suite constructs it as a flat object literal (`{path, directoryPath,
/// content}`), so this is ported as a flat struct with the same three
/// fields rather than composing a `GitIgnoreFileReference` — matching how the
/// type is actually used, not just how it's declared.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct GitIgnoreFileContent {
    pub path: String,
    pub directory_path: String,
    pub content: String,
}

/// One directory's `.gitignore` matcher. Not one of the TS file's 7 exports
/// (the `GitIgnoreRuleSet` TS interface is declared but never carries
/// `export`) — kept private here too; [`FileTreeGitIgnoreRules::rule_sets`]
/// is accordingly a private field, so this type never needs to be `pub`.
#[derive(Debug)]
struct GitIgnoreRuleSet {
    directory_path: String,
    matcher: Gitignore,
}

/// Mirrors the exported `FileTreeGitIgnoreRules` interface.
#[derive(Debug)]
pub struct FileTreeGitIgnoreRules {
    pub root_folder_path: String,
    rule_sets: Vec<GitIgnoreRuleSet>,
}

// --- exported functions, in the TS source's own order ---------------------

/// Mirrors the exported `isWorkspaceRelativePath`.
///
/// Backend file paths are workspace-relative (`web/src/x.ts`); only the
/// explorer's `rootFolderPath` (and desktop-heritage trees) use absolute
/// paths. Workspace-relative paths always belong to the active root.
#[must_use]
pub fn is_workspace_relative_path(path: &str) -> bool {
    !path.starts_with('/') && !path.starts_with('\\') && !has_drive_prefix(path)
}

/// Mirrors `isRootDirectoryPath`: the owning directory of a root-level
/// `.gitignore` in a relative tree is `''` (or `'.'`).
fn is_root_directory_path(directory_path: &str) -> bool {
    let normalized = normalize_path(&strip_trailing_path_separators(directory_path));
    normalized.is_empty() || normalized == "."
}

/// Mirrors `isPathInRootScope`.
fn is_path_in_root_scope(path: &str, root_folder_path: &str) -> bool {
    is_workspace_relative_path(path) || path_starts_with_root(path, root_folder_path)
}

/// Mirrors `pathDepth`.
fn path_depth(path: &str) -> usize {
    normalize_path(&strip_trailing_path_separators(path))
        .split('/')
        .filter(|segment| !segment.is_empty())
        .count()
}

/// Mirrors `compareIgnoreReferences`. The TS comparator takes two
/// `GitIgnoreFileReference`s but only ever reads `.directoryPath` off each —
/// narrowed to `&str` here since that's the entire contract; produces the
/// same total order (depth ascending, then normalized-path string order) via
/// `Ordering::then_with` instead of a manual subtract-and-compare, with no
/// behaviour change. Byte-wise `cmp`, not locale-aware `localeCompare` —
/// same convention `crate::git::build_git_folder_tree::sort_folders_by_name`
/// already documents for this codebase.
fn compare_directory_paths(left: &str, right: &str) -> std::cmp::Ordering {
    path_depth(left)
        .cmp(&path_depth(right))
        .then_with(|| normalize_path(left).cmp(&normalize_path(right)))
}

/// A directory-only entry point's falsy-string guard: TS's `!rootFolderPath`
/// is `true` for both `undefined` and `''`. Shared by both public
/// entry points below.
fn non_empty_root(root_folder_path: Option<&str>) -> Option<&str> {
    root_folder_path.filter(|path| !path.is_empty())
}

/// Mirrors the `isInRoot` closure defined locally inside
/// `collectGitIgnoreFileReferences` — deliberately *not* the same check as
/// [`is_path_in_root_scope`] (it only excludes a leading `/`, not a leading
/// `\` or a drive letter), ported as its own function to keep that
/// distinction rather than collapsing the two.
fn is_in_collected_root(path: &str, root_folder_path: &str) -> bool {
    !path.starts_with('/') || path_starts_with_root(path, root_folder_path)
}

/// Mirrors the exported `collectGitIgnoreFileReferences`.
///
/// Reference only the `.gitignore` files actually present in the loaded
/// tree. Deliberately does **not** synthesize a conventional root
/// `.gitignore`: the backend file tree includes dotfiles, so a real root
/// `.gitignore` is always surfaced by the walk below once the root level
/// loads (it loads first). Synthesizing one when the project root has none —
/// common for the project-home workspace, whose root is the bare project
/// directory — fetched a non-existent file and 404'd on every load.
#[must_use]
pub fn collect_git_ignore_file_references(
    files: &[GitIgnoreTreeEntry],
    root_folder_path: Option<&str>,
) -> Vec<GitIgnoreFileReference> {
    let Some(root_folder_path) = non_empty_root(root_folder_path) else {
        return Vec::new();
    };

    // Keyed on normalized path, exactly like the TS `Map`: a later walk hit
    // for the same normalized path overwrites an earlier one. Since every
    // `.gitignore` file collected here owns a distinct directory (a
    // filesystem cannot hold two files named `.gitignore` in one directory),
    // `compare_directory_paths` below produces a full order with no ties for
    // any realistic input, so this `HashMap`'s non-deterministic iteration
    // order never leaks into the sorted result.
    let mut references: HashMap<String, GitIgnoreFileReference> = HashMap::new();
    walk_collecting_gitignore_references(files, root_folder_path, &mut references);

    let mut result: Vec<GitIgnoreFileReference> = references.into_values().collect();
    result.sort_by(|a, b| compare_directory_paths(&a.directory_path, &b.directory_path));
    result
}

/// The recursive tree walk inside `collectGitIgnoreFileReferences`, pulled
/// out to its own top-level function (clippy's `items_after_statements`,
/// part of this workspace's `pedantic` gate, forbids a nested `fn` after
/// other statements in the same block).
fn walk_collecting_gitignore_references(
    entries: &[GitIgnoreTreeEntry],
    root_folder_path: &str,
    references: &mut HashMap<String, GitIgnoreFileReference>,
) {
    for entry in entries {
        if entry.name == GITIGNORE_FILE_NAME
            && !entry.is_dir
            && is_in_collected_root(&entry.path, root_folder_path)
        {
            let normalized_path = normalize_path(&strip_trailing_path_separators(&entry.path));
            references.insert(
                normalized_path,
                GitIgnoreFileReference {
                    path: entry.path.clone(),
                    directory_path: get_dir_name(&entry.path),
                },
            );
        }

        walk_collecting_gitignore_references(&entry.children, root_folder_path, references);
    }
}

/// Mirrors `addGitIgnoreContent`: keep the rest of the file usable if a
/// single malformed pattern is present, by discarding a line's error rather
/// than aborting the whole `.gitignore`.
fn add_gitignore_content(builder: &mut GitignoreBuilder, content: &str) {
    for line in content.split('\n') {
        let line = line.strip_suffix('\r').unwrap_or(line);
        let _ = builder.add_line(None, line);
    }
}

/// Mirrors the exported `createFileTreeGitIgnoreRules`.
#[must_use]
pub fn create_file_tree_git_ignore_rules(
    root_folder_path: Option<&str>,
    ignore_files: &[GitIgnoreFileContent],
) -> Option<FileTreeGitIgnoreRules> {
    let root_folder_path = non_empty_root(root_folder_path)?;
    if ignore_files.is_empty() {
        return None;
    }

    let mut filtered: Vec<&GitIgnoreFileContent> = ignore_files
        .iter()
        .filter(|file| {
            !file.directory_path.starts_with('/')
                || path_starts_with_root(&file.directory_path, root_folder_path)
        })
        .collect();
    filtered.sort_by(|a, b| compare_directory_paths(&a.directory_path, &b.directory_path));

    let mut rule_sets = Vec::with_capacity(filtered.len());
    for file in filtered {
        // Root "." disables `Gitignore`'s own prefix-stripping (see its
        // `strip()`): matcher paths handed to it below are already
        // relativized to `file.directory_path` by `to_matcher_path`, mirroring
        // the TS source's `ignore({ allowRelativePaths: true })` — "trust the
        // path I give you, don't try to re-derive a root from it."
        let mut builder = GitignoreBuilder::new(".");
        add_gitignore_content(&mut builder, &file.content);
        // `build()` can only fail if the underlying glob set fails to
        // assemble; every individual glob was already validated (and
        // discarded on failure) by `add_gitignore_content` above, so this is
        // a defensive skip, not an expected path — matches the "keep the
        // rest usable" philosophy one level up (a whole ruleset, not just
        // one line).
        let Ok(matcher) = builder.build() else {
            continue;
        };
        rule_sets.push(GitIgnoreRuleSet {
            directory_path: file.directory_path.clone(),
            matcher,
        });
    }

    if rule_sets.is_empty() {
        return None;
    }

    Some(FileTreeGitIgnoreRules {
        root_folder_path: root_folder_path.to_string(),
        rule_sets,
    })
}

/// Mirrors `toMatcherPath`. Does **not** port the trailing-slash-append the
/// TS source uses to signal "this is a directory" — see this module's own
/// doc comment for why that would be a spurious double-signal against this
/// crate's API. Directory-ness is instead carried by the caller straight
/// through to [`Gitignore::matched`]'s own `is_dir` parameter.
fn to_matcher_path(full_path: &str, directory_path: &str) -> Option<String> {
    let relative = if is_root_directory_path(directory_path) {
        if !is_workspace_relative_path(full_path) {
            return None;
        }
        full_path.to_string()
    } else if path_starts_with_root(full_path, directory_path) {
        get_relative_path(full_path, directory_path)
    } else {
        return None;
    };

    if relative.trim().is_empty() {
        return None;
    }

    Some(normalize_path(&relative))
}

/// Mirrors `isPathIgnoredByOwnRules`. Despite the name (kept from the TS
/// source), this tests `full_path` against **every** ruleset in `rules`, not
/// just the one belonging to `full_path`'s own directory — each ruleset
/// whose directory is an ancestor of (or equal to) `full_path` contributes,
/// processed shallow-to-deep (see [`compare_directory_paths`]'s sort in
/// [`create_file_tree_git_ignore_rules`]), so a deeper, more specific
/// `.gitignore` overrides a shallower one for the same path — the accumulator
/// loop below is exactly that precedence rule.
fn is_path_ignored_by_own_rules(
    rules: Option<&FileTreeGitIgnoreRules>,
    full_path: &str,
    is_dir: bool,
) -> bool {
    let Some(rules) = rules else {
        return false;
    };
    if !is_path_in_root_scope(full_path, &rules.root_folder_path) {
        return false;
    }

    let root_relative = if is_workspace_relative_path(full_path) {
        full_path.to_string()
    } else {
        get_relative_path(full_path, &rules.root_folder_path)
    };
    if root_relative.trim().is_empty() {
        return false;
    }
    let root_relative = normalize_path(&root_relative);
    if root_relative == ".git" || root_relative == ".git/" {
        return false;
    }

    let mut ignored = false;
    for rule_set in &rules.rule_sets {
        let Some(matcher_path) = to_matcher_path(full_path, &rule_set.directory_path) else {
            continue;
        };
        match rule_set.matcher.matched(&matcher_path, is_dir) {
            Match::Ignore(_) => ignored = true,
            Match::Whitelist(_) => ignored = false,
            Match::None => {}
        }
    }

    ignored
}

/// Mirrors the **local, live** `getAncestorDirectoryPaths` (TS source, line
/// 206) — not the same-named function exported from
/// `file-explorer-tree-utils.ts`, which `native/mapping/tier-a-denominator.md`
/// §5's export-level table confirms is **TEST-ONLY** (zero non-test, zero
/// self-file references; its only exerciser is that file's own dedicated
/// test). This is the copy that actually runs in production — ported
/// instead of the dead exported twin, so this crate does not carry two
/// implementations of the same ancestor-walk idea.
fn get_ancestor_directory_paths(full_path: &str, root_folder_path: &str) -> Vec<String> {
    let mut ancestors: Vec<String> = Vec::new();
    let normalized_root_path = normalize_path(&strip_trailing_path_separators(root_folder_path));
    let mut current_path = get_dir_name(full_path);

    while !current_path.is_empty() && is_path_in_root_scope(&current_path, root_folder_path) {
        if normalize_path(&strip_trailing_path_separators(&current_path)) == normalized_root_path {
            break;
        }

        ancestors.insert(0, current_path.clone());
        current_path = get_dir_name(&current_path);
    }

    ancestors
}

/// Mirrors the exported `isPathGitIgnoredByFileTreeRules` — the file's real
/// algorithmic contribution. Walks every ancestor directory and tests each
/// ancestor's own accumulated rules *before* testing the target path itself,
/// because a directory ignored by a parent rule ignores everything beneath
/// it regardless of that subdirectory's own `.gitignore`. See this module's
/// doc comment for why the `ignore` crate has no equivalent to this cascade.
#[must_use]
pub fn is_path_git_ignored_by_file_tree_rules(
    rules: Option<&FileTreeGitIgnoreRules>,
    full_path: &str,
    is_dir: bool,
) -> bool {
    let Some(unwrapped_rules) = rules else {
        return false;
    };
    if !is_path_in_root_scope(full_path, &unwrapped_rules.root_folder_path) {
        return false;
    }

    for ancestor_path in get_ancestor_directory_paths(full_path, &unwrapped_rules.root_folder_path)
    {
        if is_path_ignored_by_own_rules(rules, &ancestor_path, true) {
            return true;
        }
    }

    is_path_ignored_by_own_rules(rules, full_path, is_dir)
}

#[cfg(test)]
mod tests {
    use super::{
        GitIgnoreFileContent, GitIgnoreTreeEntry, collect_git_ignore_file_references,
        create_file_tree_git_ignore_rules, is_path_git_ignored_by_file_tree_rules,
        is_workspace_relative_path,
    };

    fn dir(name: &str, path: &str, children: Vec<GitIgnoreTreeEntry>) -> GitIgnoreTreeEntry {
        GitIgnoreTreeEntry {
            name: name.to_string(),
            path: path.to_string(),
            is_dir: true,
            children,
        }
    }

    fn file(name: &str, path: &str) -> GitIgnoreTreeEntry {
        GitIgnoreTreeEntry {
            name: name.to_string(),
            path: path.to_string(),
            is_dir: false,
            children: Vec::new(),
        }
    }

    fn ignore_file(path: &str, directory_path: &str, content: &str) -> GitIgnoreFileContent {
        GitIgnoreFileContent {
            path: path.to_string(),
            directory_path: directory_path.to_string(),
            content: content.to_string(),
        }
    }

    // --- ported from
    // web/src/__tests__/features/file-explorer/file-tree-gitignore.test.ts ---

    #[test]
    fn collects_root_and_nested_gitignore_files_from_the_loaded_tree() {
        let references = collect_git_ignore_file_references(
            &[
                file(".gitignore", "/repo/.gitignore"),
                dir(
                    "subprojects",
                    "/repo/subprojects",
                    vec![
                        file(".gitignore", "/repo/subprojects/.gitignore"),
                        dir(
                            "nested",
                            "/repo/subprojects/nested",
                            vec![file(".gitignore", "/repo/subprojects/nested/.gitignore")],
                        ),
                    ],
                ),
                file(".gitignore", "/other/.gitignore"),
            ],
            Some("/repo"),
        );

        let paths: Vec<&str> = references.iter().map(|r| r.path.as_str()).collect();
        assert_eq!(
            paths,
            vec![
                "/repo/.gitignore",
                "/repo/subprojects/.gitignore",
                "/repo/subprojects/nested/.gitignore",
            ]
        );
    }

    #[test]
    fn does_not_synthesize_a_root_gitignore_reference_when_the_loaded_tree_has_none() {
        let references = collect_git_ignore_file_references(
            &[
                dir("crowbar", "crowbar", vec![]),
                dir("desktop", "desktop", vec![]),
            ],
            Some("/repo"),
        );
        assert!(references.is_empty());
    }

    #[test]
    fn returns_no_references_for_an_empty_not_yet_loaded_tree_instead_of_synthesizing_one() {
        assert!(collect_git_ignore_file_references(&[], Some("/repo")).is_empty());
    }

    #[test]
    fn applies_nested_gitignore_files_relative_to_the_directory_that_owns_them() {
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[
                ignore_file("/repo/.gitignore", "/repo", "target/\n*.tmp\n"),
                ignore_file(
                    "/repo/subprojects/.gitignore",
                    "/repo/subprojects",
                    "/**/\npackagecache\nchumsky\n",
                ),
            ],
        );

        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/target",
            true
        ));
        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/src/file.tmp",
            false
        ));
        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/subprojects/zerocopy-0.8.48",
            true
        ));
        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/subprojects/packagecache",
            true
        ));
        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/zerocopy-0.8.48",
            true
        ));
        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/src/main.ts",
            false
        ));
    }

    #[test]
    fn lets_lower_gitignore_files_unignore_files_ignored_by_parent_rules() {
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[
                ignore_file("/repo/.gitignore", "/repo", "*.log\n"),
                ignore_file("/repo/logs/.gitignore", "/repo/logs", "!keep.log\n"),
            ],
        );

        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/logs/error.log",
            false
        ));
        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/logs/keep.log",
            false
        ));
    }

    #[test]
    fn keeps_files_ignored_when_an_ancestor_directory_is_ignored() {
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[
                ignore_file("/repo/.gitignore", "/repo", "logs/\n"),
                ignore_file("/repo/logs/.gitignore", "/repo/logs", "!keep.log\n"),
            ],
        );

        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/logs",
            true
        ));
        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/logs/keep.log",
            false
        ));
    }

    #[test]
    fn handles_anchored_nested_patterns_and_directory_only_patterns() {
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[ignore_file(
                "/repo/sub/.gitignore",
                "/repo/sub",
                "/cache\nbuild/\n",
            )],
        );

        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/sub/cache",
            false
        ));
        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/sub/deep/cache",
            false
        ));
        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/sub/build",
            true
        ));
        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/sub/build.txt",
            false
        ));
    }

    #[test]
    fn supports_windows_paths_after_normalizing_matcher_input() {
        let rules = create_file_tree_git_ignore_rules(
            Some("C:\\repo"),
            &[ignore_file(
                "C:\\repo\\sub\\.gitignore",
                "C:\\repo\\sub",
                "*.gen.ts\n",
            )],
        );

        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "C:\\repo\\sub\\client.gen.ts",
            false
        ));
        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "C:\\repo\\other\\client.gen.ts",
            false
        ));
    }

    #[test]
    fn keeps_valid_rules_when_a_gitignore_contains_a_malformed_line() {
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[ignore_file(
                "/repo/.gitignore",
                "/repo",
                "valid.out\nbad\\\n\\#literal-hash\n",
            )],
        );

        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/valid.out",
            false
        ));
        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/#literal-hash",
            false
        ));
        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/other/valid.out",
            false
        ));
    }

    mod workspace_relative_paths_synthetic_root {
        use super::{
            collect_git_ignore_file_references, create_file_tree_git_ignore_rules, dir, file,
            ignore_file, is_path_git_ignored_by_file_tree_rules,
        };

        const ROOT_FOLDER_PATH: &str = "/repos/repo-1";

        #[test]
        fn collects_workspace_relative_gitignore_files_under_a_synthetic_root() {
            let references = collect_git_ignore_file_references(
                &[
                    file(".gitignore", ".gitignore"),
                    dir("web", "web", vec![file(".gitignore", "web/.gitignore")]),
                    file(".gitignore", "/other/.gitignore"),
                ],
                Some(ROOT_FOLDER_PATH),
            );

            let paths: Vec<&str> = references.iter().map(|r| r.path.as_str()).collect();
            assert_eq!(paths, vec![".gitignore", "web/.gitignore"]);
            assert_eq!(references[0].directory_path, "");
        }

        // TS: `it.each(['', '.'])`.
        #[test]
        fn matches_relative_tree_paths_against_a_root_gitignore_with_directory_path_empty_string() {
            matches_relative_tree_paths_against_a_root_gitignore("");
        }

        #[test]
        fn matches_relative_tree_paths_against_a_root_gitignore_with_directory_path_dot() {
            matches_relative_tree_paths_against_a_root_gitignore(".");
        }

        fn matches_relative_tree_paths_against_a_root_gitignore(directory_path: &str) {
            let rules = create_file_tree_git_ignore_rules(
                Some(ROOT_FOLDER_PATH),
                &[ignore_file(".gitignore", directory_path, "node_modules\n")],
            );

            assert!(is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "node_modules",
                true
            ));
            assert!(is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "node_modules/x",
                false
            ));
            assert!(!is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "web",
                true
            ));
            assert!(!is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "web/src/x.ts",
                false
            ));
        }

        #[test]
        fn keeps_relative_paths_ignored_when_an_ignored_ancestor_directory_contains_them() {
            let rules = create_file_tree_git_ignore_rules(
                Some(ROOT_FOLDER_PATH),
                &[
                    ignore_file(".gitignore", "", "dist/\n"),
                    ignore_file("dist/.gitignore", "dist", "!keep.txt\n"),
                ],
            );

            assert!(is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "dist",
                true
            ));
            assert!(is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "dist/keep.txt",
                false
            ));
        }

        #[test]
        fn applies_nested_workspace_relative_gitignore_files_to_their_own_directory() {
            let rules = create_file_tree_git_ignore_rules(
                Some(ROOT_FOLDER_PATH),
                &[ignore_file("web/.gitignore", "web", "*.gen.ts\n")],
            );

            assert!(is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "web/client.gen.ts",
                false
            ));
            assert!(!is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "api/client.gen.ts",
                false
            ));
        }

        #[test]
        fn lets_nested_relative_gitignore_files_unignore_parent_rules() {
            let rules = create_file_tree_git_ignore_rules(
                Some(ROOT_FOLDER_PATH),
                &[
                    ignore_file(".gitignore", "", "*.log\n"),
                    ignore_file("logs/.gitignore", "logs", "!keep.log\n"),
                ],
            );

            assert!(is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "logs/error.log",
                false
            ));
            assert!(!is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "logs/keep.log",
                false
            ));
        }

        #[test]
        fn does_not_treat_the_relative_git_directory_as_ignored() {
            let rules = create_file_tree_git_ignore_rules(
                Some(ROOT_FOLDER_PATH),
                &[ignore_file(".gitignore", "", ".git\n*\n")],
            );

            assert!(!is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                ".git",
                true
            ));
            assert!(is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "file.txt",
                false
            ));
        }

        #[test]
        fn does_not_match_absolute_paths_against_a_root_relative_rule_set() {
            let rules = create_file_tree_git_ignore_rules(
                Some(ROOT_FOLDER_PATH),
                &[ignore_file(".gitignore", "", "node_modules\n")],
            );

            assert!(!is_path_git_ignored_by_file_tree_rules(
                rules.as_ref(),
                "/other/node_modules",
                true
            ));
        }
    }

    #[test]
    fn does_not_treat_the_repository_root_or_git_directory_as_ignored() {
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[ignore_file("/repo/.gitignore", "/repo", ".git\n*\n")],
        );

        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo",
            true
        ));
        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/.git",
            true
        ));
        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/file.txt",
            false
        ));
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn is_workspace_relative_path_rejects_leading_slash_backslash_and_drive_letter() {
        assert!(is_workspace_relative_path("web/src/x.ts"));
        assert!(!is_workspace_relative_path("/repo/x.ts"));
        assert!(!is_workspace_relative_path("\\repo\\x.ts"));
        assert!(!is_workspace_relative_path("C:\\repo\\x.ts"));
    }

    #[test]
    fn create_file_tree_git_ignore_rules_returns_none_for_an_empty_ignore_file_list() {
        assert!(create_file_tree_git_ignore_rules(Some("/repo"), &[]).is_none());
    }

    #[test]
    fn create_file_tree_git_ignore_rules_returns_none_when_root_folder_path_is_absent() {
        assert!(
            create_file_tree_git_ignore_rules(
                None,
                &[ignore_file("/repo/.gitignore", "/repo", "*.log\n")]
            )
            .is_none()
        );
    }

    #[test]
    fn create_file_tree_git_ignore_rules_returns_none_when_every_ignore_file_is_outside_root_scope()
    {
        // Every candidate's `directoryPath` is absolute and outside `/repo`,
        // so the root-scope filter drops all of them and `ruleSets` ends up
        // empty — the `if (ruleSets.length === 0) return null` branch, not
        // the earlier `ignoreFiles.length === 0` one.
        assert!(
            create_file_tree_git_ignore_rules(
                Some("/repo"),
                &[ignore_file("/other/.gitignore", "/other", "*.log\n")]
            )
            .is_none()
        );
    }

    #[test]
    fn is_path_git_ignored_by_file_tree_rules_returns_false_for_a_path_outside_root_scope() {
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[ignore_file("/repo/.gitignore", "/repo", "*.log\n")],
        );

        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/elsewhere/app.log",
            false
        ));
    }

    #[test]
    fn is_path_git_ignored_by_file_tree_rules_returns_false_when_rules_is_none() {
        assert!(!is_path_git_ignored_by_file_tree_rules(
            None,
            "/repo/app.log",
            false
        ));
    }

    #[test]
    fn collect_git_ignore_file_references_returns_empty_when_root_folder_path_is_absent() {
        assert!(
            collect_git_ignore_file_references(&[file(".gitignore", "/repo/.gitignore")], None)
                .is_empty()
        );
    }

    #[test]
    fn a_workspace_relative_ruleset_directory_does_not_match_against_an_absolute_full_path() {
        // `directoryPath: ""` (a "root directory path") paired with an
        // *absolute* `rootFolderPath` is a mismatched-convention input the
        // type signature allows but the two production call sites never
        // actually construct. `toMatcherPath`'s own
        // `!isWorkspaceRelativePath(fullPath) -> null` guard makes this a
        // silent no-match rather than a panic or an accidental match —
        // verified directly, not just reasoned about.
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[ignore_file(".gitignore", "", "*.log\n")],
        );

        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/app.log",
            false
        ));
    }

    // --- ancestor-cascade coverage: the file's own real contribution ---
    //
    // The TS suite's "keeps files ignored when an ancestor directory is
    // ignored" case already exercises the cascade once; these add depth-2+
    // and multi-sibling coverage the brief calls out as "the part worth
    // testing hardest."

    #[test]
    fn cascade_ignores_a_file_two_levels_beneath_an_ignored_ancestor() {
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[ignore_file("/repo/.gitignore", "/repo", "build/\n")],
        );

        assert!(is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/build/nested/deep/output.js",
            false
        ));
    }

    #[test]
    fn cascade_does_not_ignore_a_sibling_directory_of_an_ignored_one() {
        let rules = create_file_tree_git_ignore_rules(
            Some("/repo"),
            &[ignore_file("/repo/.gitignore", "/repo", "build/\n")],
        );

        assert!(!is_path_git_ignored_by_file_tree_rules(
            rules.as_ref(),
            "/repo/src/main.ts",
            false
        ));
    }
}

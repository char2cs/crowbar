//! Flat [`GitFileDTO`] list -> nested folder tree, for the sidebar
//! changed-files tree.
//!
//! Ported from `web/src/features/git/utils/build-git-folder-tree.ts`.
//!
//! # A type note: `GitFileDTO`, not `GitDiff`
//!
//! The TS function's *declared* signature takes `GitFile[]`
//! ([`crowbar_proto::api_v0_dto::GitFileDTO`] here — see
//! `crate::git::types` for why that DTO, not a hand-rolled duplicate, is the
//! right match). Its one production call site
//! (`web/src/features/git/components/changed-files-tree.tsx`) actually feeds
//! it `GitDiff[]` objects widened with extra display fields and cast back to
//! `GitFile[]` — a structural-typing trick TS allows and Rust does not. That
//! widening exists so the tree can carry `additions`/`deletions`/
//! `uncommitted`/a second `filePath` alias through to the row renderer
//! alongside each file; it is presentation glue local to that one component,
//! not part of what `buildGitFolderTree` itself computes (the function body
//! only ever reads `.path` off each element — see `build_git_folder_tree`
//! below). This port keeps the function's own declared, tested contract
//! (`GitFileDTO` in, `GitFileDTO` out) and leaves the "carry extra display
//! fields through the tree" concern to whatever Phase-4/`crowbar-ui` code
//! eventually builds the real sidebar row list.

use std::collections::HashMap;

use crowbar_proto::api_v0_dto::GitFileDTO;

/// One folder in the tree built by [`build_git_folder_tree`].
///
/// `folders` is a [`HashMap`], not the TS source's insertion-ordered `Map`:
/// nothing that reads a `GitFolderNode` relies on child order without going
/// through [`sort_folders_by_name`] first — that function exists precisely
/// because raw insertion order is never display order. [`collect_node_files`]
/// likewise never asserts an order (both its ported TS test cases sort or
/// length-check the result rather than comparing it positionally).
#[derive(Debug, Clone, Default)]
pub struct GitFolderNode {
    pub name: String,
    pub full_path: String,
    pub folders: HashMap<String, GitFolderNode>,
    pub files: Vec<GitFileDTO>,
}

/// Mirrors `createFolderNode`.
#[must_use]
pub fn create_folder_node(name: &str, full_path: &str) -> GitFolderNode {
    GitFolderNode {
        name: name.to_string(),
        full_path: full_path.to_string(),
        folders: HashMap::new(),
        files: Vec::new(),
    }
}

/// Split a path into non-empty segments, normalising `\` to `/` first.
/// Mirrors `normalizePathSegments`.
#[must_use]
pub fn normalize_path_segments(path: &str) -> Vec<String> {
    path.replace('\\', "/")
        .split('/')
        .map(str::trim)
        .filter(|segment| !segment.is_empty())
        .map(str::to_string)
        .collect()
}

/// Build the nested folder tree for `file_list`. Only `file.path` is ever
/// read — every other field rides along untouched in the leaf `files`
/// vectors, exactly like the TS source (see the module doc for the one place
/// that matters). Mirrors `buildGitFolderTree`.
#[must_use]
pub fn build_git_folder_tree(file_list: &[GitFileDTO]) -> GitFolderNode {
    let mut root = create_folder_node("", "");

    for file in file_list {
        let segments = normalize_path_segments(&file.path);
        if segments.is_empty() {
            continue;
        }

        let mut current = &mut root;
        let mut current_path = String::new();
        let (directory_segments, _file_segment) = segments.split_at(segments.len() - 1);
        for segment in directory_segments {
            current_path = if current_path.is_empty() {
                segment.clone()
            } else {
                format!("{current_path}/{segment}")
            };
            current = current
                .folders
                .entry(segment.clone())
                .or_insert_with(|| create_folder_node(segment, &current_path));
        }

        current.files.push(file.clone());
    }

    root
}

/// Sort a collection of folder-node references alphabetically by name.
/// Non-mutating: returns a new `Vec` of borrows, the source is untouched.
/// Mirrors `sortFoldersByName` (ASCII/byte-wise `cmp`, not locale-aware
/// `localeCompare` — every folder name this crate builds comes from a
/// filesystem path segment, which `crowbar-core` treats as opaque bytes
/// elsewhere too, e.g. `workspace::scope_url`'s hand-rolled URI encoder).
#[must_use]
pub fn sort_folders_by_name<'a, I>(folders: I) -> Vec<&'a GitFolderNode>
where
    I: IntoIterator<Item = &'a GitFolderNode>,
{
    let mut sorted: Vec<&'a GitFolderNode> = folders.into_iter().collect();
    sorted.sort_by(|a, b| a.name.cmp(&b.name));
    sorted
}

/// Sort a copy of `file_list` by path, leaving the input untouched. Mirrors
/// `sortFilesByPath`.
#[must_use]
pub fn sort_files_by_path(file_list: &[GitFileDTO]) -> Vec<GitFileDTO> {
    let mut sorted = file_list.to_vec();
    sorted.sort_by(|a, b| a.path.cmp(&b.path));
    sorted
}

/// Every file under `node`, recursively, own files first then each child
/// folder's files. Order across folders is unspecified (see the
/// [`GitFolderNode`] doc). Mirrors `collectNodeFiles`.
#[must_use]
pub fn collect_node_files(node: &GitFolderNode) -> Vec<GitFileDTO> {
    let mut out = node.files.clone();
    for child in node.folders.values() {
        out.extend(collect_node_files(child));
    }
    out
}

#[cfg(test)]
mod tests {
    use super::{
        build_git_folder_tree, collect_node_files, create_folder_node, normalize_path_segments,
        sort_files_by_path, sort_folders_by_name,
    };
    use crowbar_proto::api_v0_dto::GitFileDTO;

    fn make_file(path: &str) -> GitFileDTO {
        GitFileDTO {
            path: path.to_string(),
            status: "modified".to_string(),
            staged: false,
        }
    }

    // --- ported from web/src/__tests__/features/git/utils/build-git-folder-tree.test.ts ---

    #[test]
    fn normalize_path_segments_splits_a_unix_path_into_segments() {
        assert_eq!(
            normalize_path_segments("src/sub/b.ts"),
            vec!["src", "sub", "b.ts"]
        );
    }

    #[test]
    fn normalize_path_segments_normalizes_backslashes_to_forward_slashes() {
        assert_eq!(
            normalize_path_segments("src\\sub\\b.ts"),
            vec!["src", "sub", "b.ts"]
        );
    }

    #[test]
    fn normalize_path_segments_strips_empty_segments_from_leading_trailing_slashes() {
        assert_eq!(normalize_path_segments("/src/a.ts/"), vec!["src", "a.ts"]);
    }

    #[test]
    fn normalize_path_segments_returns_empty_vec_for_empty_string() {
        assert_eq!(normalize_path_segments(""), Vec::<String>::new());
    }

    #[test]
    fn create_folder_node_creates_a_node_with_correct_shape() {
        let node = create_folder_node("src", "src");
        assert_eq!(node.name, "src");
        assert_eq!(node.full_path, "src");
        assert!(node.folders.is_empty());
        assert!(node.files.is_empty());
    }

    #[test]
    fn places_root_level_files_at_the_root_node() {
        let root = build_git_folder_tree(&[make_file("README.md")]);
        assert_eq!(root.files.len(), 1);
        assert_eq!(root.files[0].path, "README.md");
        assert!(root.folders.is_empty());
    }

    #[test]
    fn nests_src_a_ts_under_src_folder() {
        let root = build_git_folder_tree(&[make_file("src/a.ts")]);
        let src_node = root.folders.get("src").expect("src folder is present");
        assert_eq!(src_node.files.len(), 1);
        assert_eq!(src_node.files[0].path, "src/a.ts");
    }

    #[test]
    fn nests_src_sub_b_ts_under_src_sub() {
        let root = build_git_folder_tree(&[make_file("src/sub/b.ts")]);
        let src_node = root.folders.get("src").expect("src folder is present");
        let sub_node = src_node.folders.get("sub").expect("sub folder is present");
        assert_eq!(sub_node.files.len(), 1);
        assert_eq!(sub_node.files[0].path, "src/sub/b.ts");
    }

    #[test]
    fn builds_the_full_tree_src_nests_sub_readme_at_root() {
        let root = build_git_folder_tree(&[
            make_file("src/a.ts"),
            make_file("src/sub/b.ts"),
            make_file("README.md"),
        ]);

        assert_eq!(root.files.len(), 1);
        assert_eq!(root.files[0].path, "README.md");

        let src_node = root.folders.get("src").expect("src folder is present");
        assert_eq!(src_node.name, "src");
        assert_eq!(src_node.full_path, "src");
        assert_eq!(src_node.files.len(), 1);
        assert_eq!(src_node.files[0].path, "src/a.ts");

        let sub_node = src_node.folders.get("sub").expect("sub folder is present");
        assert_eq!(sub_node.name, "sub");
        assert_eq!(sub_node.full_path, "src/sub");
        assert_eq!(sub_node.files.len(), 1);
        assert_eq!(sub_node.files[0].path, "src/sub/b.ts");
    }

    #[test]
    fn returns_empty_root_for_empty_input() {
        let root = build_git_folder_tree(&[]);
        assert!(root.files.is_empty());
        assert!(root.folders.is_empty());
    }

    #[test]
    fn skips_files_with_empty_paths() {
        let root = build_git_folder_tree(&[make_file(""), make_file("a.ts")]);
        assert_eq!(root.files.len(), 1);
        assert_eq!(root.files[0].path, "a.ts");
    }

    #[test]
    fn shares_the_same_folder_node_for_sibling_files() {
        let root = build_git_folder_tree(&[make_file("src/a.ts"), make_file("src/b.ts")]);
        let src_node = root.folders.get("src").expect("src folder is present");
        assert_eq!(src_node.files.len(), 2);
    }

    #[test]
    fn sort_folders_by_name_sorts_folders_alphabetically_by_name() {
        let folders = [
            create_folder_node("z-folder", "z-folder"),
            create_folder_node("a-folder", "a-folder"),
            create_folder_node("m-folder", "m-folder"),
        ];
        let sorted = sort_folders_by_name(folders.iter());
        let names: Vec<&str> = sorted.iter().map(|f| f.name.as_str()).collect();
        assert_eq!(names, vec!["a-folder", "m-folder", "z-folder"]);
    }

    #[test]
    fn sort_folders_by_name_works_with_a_hash_map_values_iterator() {
        let root = build_git_folder_tree(&[make_file("z/a.ts"), make_file("a/b.ts")]);
        let sorted = sort_folders_by_name(root.folders.values());
        assert_eq!(sorted[0].name, "a");
        assert_eq!(sorted[1].name, "z");
    }

    #[test]
    fn sort_files_by_path_sorts_files_by_path() {
        let files = vec![make_file("z.ts"), make_file("a.ts"), make_file("m.ts")];
        let sorted = sort_files_by_path(&files);
        let paths: Vec<&str> = sorted.iter().map(|f| f.path.as_str()).collect();
        assert_eq!(paths, vec!["a.ts", "m.ts", "z.ts"]);
    }

    #[test]
    fn sort_files_by_path_does_not_mutate_the_original_slice() {
        let files = vec![make_file("b.ts"), make_file("a.ts")];
        let _ = sort_files_by_path(&files);
        assert_eq!(files[0].path, "b.ts");
    }

    #[test]
    fn collect_node_files_collects_all_files_from_nested_tree() {
        let root = build_git_folder_tree(&[
            make_file("src/a.ts"),
            make_file("src/sub/b.ts"),
            make_file("README.md"),
        ]);
        let src_node = root.folders.get("src").expect("src folder is present");
        let mut collected: Vec<String> = collect_node_files(src_node)
            .into_iter()
            .map(|f| f.path)
            .collect();
        collected.sort();
        assert_eq!(collected, vec!["src/a.ts", "src/sub/b.ts"]);
    }

    #[test]
    fn collect_node_files_collects_all_files_from_root() {
        let root = build_git_folder_tree(&[
            make_file("src/a.ts"),
            make_file("src/sub/b.ts"),
            make_file("README.md"),
        ]);
        assert_eq!(collect_node_files(&root).len(), 3);
    }

    #[test]
    fn collect_node_files_returns_empty_vec_for_leaf_node_with_no_files() {
        let node = create_folder_node("empty", "empty");
        assert!(collect_node_files(&node).is_empty());
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn a_deeply_nested_path_builds_every_intermediate_folder_with_the_right_full_path() {
        let root = build_git_folder_tree(&[make_file("a/b/c/d.ts")]);
        let a = root.folders.get("a").expect("a is present");
        assert_eq!(a.full_path, "a");
        let b = a.folders.get("b").expect("b is present");
        assert_eq!(b.full_path, "a/b");
        let c = b.folders.get("c").expect("c is present");
        assert_eq!(c.full_path, "a/b/c");
        assert_eq!(c.files[0].path, "a/b/c/d.ts");
    }
}

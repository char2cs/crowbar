//! File-tree row density — ported from `web/src/features/file-explorer/
//! file-explorer/lib/file-tree-density.ts`.
//!
//! Scope, per this item's brief: [`FileTreeDensity`], [`is_file_tree_density`],
//! [`normalize_file_tree_density`], [`DEFAULT_FILE_TREE_DENSITY`], and
//! `FILE_TREE_DENSITY_CONFIG`'s `rowHeight` field only, via [`row_height`].
//! Not ported: `FILE_TREE_DENSITY_CONFIG`'s `rowClassName` field (a Tailwind
//! string — presentation) and `FILE_TREE_DENSITY_OPTIONS` (Settings-tab
//! dropdown label copy — presentation, and not reachable outside the
//! Settings dialog either).
//!
//! # This module is the file-tree/settings reconciliation
//! # `crate::settings::types::FileTreeDensity`'s own doc asked for
//!
//! That module's doc (written during the settings-schema item, before this
//! one existed) flagged its `FileTreeDensity`/`normalize_file_tree_density`
//! as "a narrow, deliberate duplicate ... when file-tree model is ported,
//! this type should be deleted in favour of that module's, and every
//! reference here repointed." This module is that port:
//! [`FileTreeDensity`]/[`normalize_file_tree_density`] here are now the one
//! definition in the crate; `crate::settings::types` re-exports them (`pub
//! use crate::file_tree::density::{...}`) rather than declaring its own
//! copy — see that module's updated doc.

/// Mirrors `FileTreeDensity` (`'compact' | 'default' | 'comfortable'`).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum FileTreeDensity {
    Compact,
    Default,
    Comfortable,
}

/// Mirrors `DEFAULT_FILE_TREE_DENSITY`.
pub const DEFAULT_FILE_TREE_DENSITY: FileTreeDensity = FileTreeDensity::Default;

/// Mirrors `isFileTreeDensity`.
#[must_use]
pub fn is_file_tree_density(value: &str) -> bool {
    matches!(value, "compact" | "default" | "comfortable")
}

/// Mirrors `normalizeFileTreeDensity`. Case-sensitive, matching the TS
/// source's own strict-equality membership check — `"Compact"` is not
/// `"compact"` and falls back to the default like any other unknown string.
#[must_use]
pub fn normalize_file_tree_density(value: &str) -> FileTreeDensity {
    match value {
        "compact" => FileTreeDensity::Compact,
        "comfortable" => FileTreeDensity::Comfortable,
        // "default" and every other input (missing/unknown) share the same
        // fallback — clippy flags a redundant explicit "default" arm
        // identical to the wildcard, matching
        // `crate::settings::normalization::normalize_render_whitespace`'s
        // precedent for the same shape.
        _ => DEFAULT_FILE_TREE_DENSITY,
    }
}

/// `FILE_TREE_DENSITY_CONFIG[density].rowHeight`, in px.
/// `FILE_TREE_DENSITY_CONFIG`'s other field, `rowClassName`, is presentation
/// and is not ported — see this module's doc.
#[must_use]
pub fn row_height(density: FileTreeDensity) -> u32 {
    match density {
        FileTreeDensity::Compact => 20,
        FileTreeDensity::Default => 24,
        FileTreeDensity::Comfortable => 28,
    }
}

#[cfg(test)]
mod tests {
    use super::{
        DEFAULT_FILE_TREE_DENSITY, FileTreeDensity, is_file_tree_density,
        normalize_file_tree_density, row_height,
    };

    // --- ported from web/src/__tests__/features/settings/settings-normalization.test.ts
    //     ("normalizes unsupported file tree density values" — that test
    //     calls normalizeSettingValue('fileTreeDensity', ...), which for
    //     that field is a pure pass-through to normalizeFileTreeDensity;
    //     already ported once as crate::settings::types's local duplicate,
    //     kept here now that this is the canonical module) ---

    #[test]
    fn accepts_a_known_density_unchanged() {
        assert_eq!(
            normalize_file_tree_density("compact"),
            FileTreeDensity::Compact
        );
        assert_eq!(
            normalize_file_tree_density("default"),
            FileTreeDensity::Default
        );
        assert_eq!(
            normalize_file_tree_density("comfortable"),
            FileTreeDensity::Comfortable
        );
    }

    #[test]
    fn falls_back_to_default_for_an_unknown_density() {
        assert_eq!(
            normalize_file_tree_density("dense"),
            FileTreeDensity::Default
        );
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn falls_back_to_default_for_an_empty_string() {
        assert_eq!(normalize_file_tree_density(""), FileTreeDensity::Default);
    }

    #[test]
    fn is_case_sensitive_matching_the_ts_strict_equality_checks() {
        assert_eq!(
            normalize_file_tree_density("Compact"),
            FileTreeDensity::Default
        );
    }

    #[test]
    fn is_file_tree_density_agrees_with_the_normalizer_on_membership() {
        for value in ["compact", "default", "comfortable"] {
            assert!(is_file_tree_density(value));
        }
        for value in ["dense", "", "Compact"] {
            assert!(!is_file_tree_density(value));
        }
    }

    #[test]
    fn default_file_tree_density_constant_is_the_default_variant() {
        assert_eq!(DEFAULT_FILE_TREE_DENSITY, FileTreeDensity::Default);
    }

    #[test]
    fn row_height_matches_the_ts_config_table() {
        assert_eq!(row_height(FileTreeDensity::Compact), 20);
        assert_eq!(row_height(FileTreeDensity::Default), 24);
        assert_eq!(row_height(FileTreeDensity::Comfortable), 28);
    }
}

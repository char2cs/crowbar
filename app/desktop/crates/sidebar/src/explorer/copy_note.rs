//! The note a Files row shows for a moment after its path is copied.

/// Which path a copy put on the clipboard, as develop's row feedback names it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CopyNote {
    /// The absolute path.
    Path,
    /// The path relative to the workspace root.
    RelativePath,
}

impl CopyNote {
    /// The words the row shows.
    // SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/components/file-explorer-tree-item.tsx
    #[must_use]
    pub const fn text(self) -> &'static str {
        match self {
            Self::Path => "✓ path copied",
            Self::RelativePath => "✓ rel copied",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_copy_names_the_path_it_copied() {
        let said = [CopyNote::Path, CopyNote::RelativePath].map(CopyNote::text);
        assert_eq!(said, ["✓ path copied", "✓ rel copied"]);
    }
}

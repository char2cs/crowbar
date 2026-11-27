#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CopyNote {
    Path,
    RelativePath,
}

impl CopyNote {
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

//! The text attributes that change how a glyph is drawn.

/// A set of glyph attributes: bold, italic and strike-through.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Hash)]
pub struct StyleFlags(u8);

impl StyleFlags {
    /// No attribute.
    pub const NONE: Self = Self(0);
    /// Heavier weight.
    pub const BOLD: Self = Self(1);
    /// Slanted.
    pub const ITALIC: Self = Self(1 << 1);
    /// A line through the middle of the glyph.
    pub const STRIKE: Self = Self(1 << 2);

    /// True when every attribute of `other` is set.
    pub fn contains(
        self,
        other: Self,
    ) -> bool {
        self.0 & other.0 == other.0
    }

    /// The union of both sets.
    #[must_use]
    pub fn with(
        self,
        other: Self,
    ) -> Self {
        Self(self.0 | other.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn union_contains_both_parts() {
        let flags = StyleFlags::BOLD.with(StyleFlags::STRIKE);
        assert!(flags.contains(StyleFlags::BOLD));
        assert!(flags.contains(StyleFlags::STRIKE));
        assert!(!flags.contains(StyleFlags::ITALIC));
        assert!(StyleFlags::NONE.contains(StyleFlags::NONE));
    }
}

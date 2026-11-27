#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Hash)]
pub struct StyleFlags(u8);

impl StyleFlags {
    pub const NONE: Self = Self(0);
    pub const BOLD: Self = Self(1);
    pub const ITALIC: Self = Self(1 << 1);
    pub const STRIKE: Self = Self(1 << 2);

    pub fn contains(
        self,
        other: Self,
    ) -> bool {
        self.0 & other.0 == other.0
    }

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

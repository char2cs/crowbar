//! A retired default of a number setting.

/// A default a number setting no longer has, and the default that replaces it on load.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct LegacyNumber {
    /// The value that used to be the default.
    pub retired: f64,
    /// The default it reads as.
    pub current: f64,
}

impl LegacyNumber {
    /// `value`, or the current default when it is exactly the retired one.
    #[must_use]
    pub fn loaded(
        self,
        value: f64,
    ) -> f64 {
        if value.total_cmp(&self.retired).is_ne() {
            return value;
        }
        self.current
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const LINE_HEIGHT: LegacyNumber = LegacyNumber {
        retired: 1.2,
        current: 1.0,
    };

    #[test]
    fn only_the_retired_default_is_replaced() {
        assert!((LINE_HEIGHT.loaded(1.2) - 1.0).abs() < f64::EPSILON);
        assert!((LINE_HEIGHT.loaded(1.3) - 1.3).abs() < f64::EPSILON);
    }
}

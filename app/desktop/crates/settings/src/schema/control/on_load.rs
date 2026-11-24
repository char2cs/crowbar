//! What develop's settings store does to a stored number when it loads it.

use crate::schema::control::legacy_number::LegacyNumber;

/// How a number setting's stored value reads on load, per develop's `normalizeSettings`.
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum OnLoad {
    /// Read as stored.
    Kept,
    /// Rounded, snapped and clamped into the setting's range.
    Normalized,
    /// A retired default reads as the current one; any other value is kept.
    Replaced(LegacyNumber),
}

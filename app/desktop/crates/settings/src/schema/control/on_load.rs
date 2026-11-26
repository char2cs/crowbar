//! What develop's settings store does to a stored number when it loads it.

use crate::schema::control::legacy_number::LegacyNumber;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) enum OnLoad {
    Kept,
    Normalized,
    Replaced(LegacyNumber),
}

use serde::{Deserialize, Serialize};

/// A Tailwind breakpoint as the pane wire names it (`sm`, `md`, `lg`, `xl`). The shell owns the
/// widths (`ui::Breakpoint`) and tells each pane which of these the app window clears.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum Breakpoint {
    /// `sm`.
    Sm,
    /// `md`.
    Md,
    /// `lg`.
    Lg,
    /// `xl`.
    Xl,
}

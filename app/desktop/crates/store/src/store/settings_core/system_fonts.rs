//! The font families of this computer, as the shell's text system reports them.

pub(crate) mod choices;

/// Every family, and the ones whose letters all share one advance width.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct SystemFonts {
    /// Every installed family.
    pub all: Vec<String>,
    /// The monospace families among them.
    pub monospace: Vec<String>,
}

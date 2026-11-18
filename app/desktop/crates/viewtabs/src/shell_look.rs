/// How a tab's pill is painted.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ShellLook {
    /// Not showing: no fill, muted text, a hover fill on pointer enter.
    Idle,
    /// The showing tab: the raised near-white pill with dark text.
    Active,
    /// The ghost of a dragged tab: the pill lifted with a stronger shadow.
    Lifted,
    /// A tab a dropped chat would merge into.
    MergeTarget,
}

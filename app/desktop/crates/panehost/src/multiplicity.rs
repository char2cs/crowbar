/// How many instances of a pane type may exist.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Multiplicity {
    /// At most one instance.
    Single,
    /// One instance per distinct set of params.
    PerParams,
    /// Any number of instances.
    Many,
}

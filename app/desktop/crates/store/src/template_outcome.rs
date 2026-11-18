/// What making an env template did.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TemplateOutcome {
    /// The template was written to this workspace-relative path.
    Created(String),
    /// Nothing was written: this path exists and the caller did not allow replacing it.
    Exists(String),
}

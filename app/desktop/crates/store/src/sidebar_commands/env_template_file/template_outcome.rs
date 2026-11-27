#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TemplateOutcome {
    Created(String),
    Exists(String),
}

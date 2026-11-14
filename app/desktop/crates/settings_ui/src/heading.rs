//! A page-level title with an explanation.

/// The large heading some pages draw above their body.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Heading {
    pub(crate) title: String,
    pub(crate) description: String,
}

impl Heading {
    /// A heading with its description.
    pub fn new(title: impl Into<String>, description: impl Into<String>) -> Self {
        Self {
            title: title.into(),
            description: description.into(),
        }
    }
}

#[cfg(test)]
#[path = "heading_tests.rs"]
mod tests;

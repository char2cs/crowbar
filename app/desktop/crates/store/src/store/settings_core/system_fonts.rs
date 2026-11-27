pub(crate) mod choices;

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct SystemFonts {
    pub all: Vec<String>,
    pub monospace: Vec<String>,
}

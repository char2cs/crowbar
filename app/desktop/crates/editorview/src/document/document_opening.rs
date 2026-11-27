#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DocumentOpening {
    pub text: String,
    pub path: String,
    pub saved: Option<String>,
}

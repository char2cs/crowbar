/// What a document opens on: its text, its path, and what the file held when the text is a draft.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DocumentOpening {
    /// The document's text.
    pub text: String,
    /// The path the toolbar's breadcrumb shows and the language comes from, or an untitled
    /// document's `untitled:` name.
    pub path: String,
    /// What the file held when `text` is an unsaved draft of it: the document opens dirty. `None`
    /// when `text` is the file's own.
    pub saved: Option<String>,
}

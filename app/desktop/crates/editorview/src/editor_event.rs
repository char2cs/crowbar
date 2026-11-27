#[derive(Debug, Clone, PartialEq, Eq)]
pub enum EditorEvent {
    DirtyChanged(bool),
    SaveRequested,
    EditsPaused,
    PreviewRequested,
    RichViewRequested,
    Definition {
        path: String,
        line: u32,
        character: u32,
    },
    DefinitionOutside(String),
}

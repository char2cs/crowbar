#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct FileFilter {
    pub name: Option<&'static str>,
    pub extensions: &'static [&'static str],
    pub mime_types: &'static [&'static str],
}

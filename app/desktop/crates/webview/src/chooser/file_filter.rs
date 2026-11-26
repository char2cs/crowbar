/// The files a chooser lists: those with one of `extensions` or of one of `mime_types`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct FileFilter {
    /// The name in the chooser's filter menu; `None` filters with no menu, as a page's `accept`.
    pub name: Option<&'static str>,
    /// File name extensions, without the dot.
    pub extensions: &'static [&'static str],
    /// MIME types.
    pub mime_types: &'static [&'static str],
}

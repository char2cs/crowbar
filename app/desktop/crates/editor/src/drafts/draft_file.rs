/// Where the drafts are kept between runs.
pub trait DraftFile {
    /// The file's text; `None` when there is none yet.
    ///
    /// # Errors
    /// When the file exists and cannot be read.
    fn read(&self) -> std::io::Result<Option<String>>;

    /// Replaces the file's text.
    ///
    /// # Errors
    /// When the file cannot be written.
    fn write(
        &self,
        text: &str,
    ) -> std::io::Result<()>;
}

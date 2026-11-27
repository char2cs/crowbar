pub trait DraftFile {
    fn read(&self) -> std::io::Result<Option<String>>;

    fn write(
        &self,
        text: &str,
    ) -> std::io::Result<()>;
}

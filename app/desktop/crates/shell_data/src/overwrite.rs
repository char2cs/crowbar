/// Whether a file that already exists may be replaced.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Overwrite {
    /// Stop and report the file, so the user can be asked.
    Ask,
    /// Replace it.
    Replace,
}

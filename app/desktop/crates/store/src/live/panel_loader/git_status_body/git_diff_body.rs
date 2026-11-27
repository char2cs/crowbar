use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
pub(crate) struct GitDiffBody {
    pub(crate) file_path: String,
    pub(crate) additions: u32,
    pub(crate) deletions: u32,
}

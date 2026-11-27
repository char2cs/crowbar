#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct ImportScope {
    pub(crate) host: String,
    pub(crate) project: String,
    pub(crate) repo: String,
    pub(crate) default_branch: String,
}

use protocol::RelayScope;

pub(crate) struct Admission {
    pub(crate) scope: RelayScope,
    pub(crate) path_and_query: String,
}

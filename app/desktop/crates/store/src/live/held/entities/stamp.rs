pub(crate) struct Stamp<'a> {
    pub(crate) kind: &'a str,
    pub(crate) id: &'a str,
    pub(crate) version: u64,
    pub(crate) gone: bool,
}

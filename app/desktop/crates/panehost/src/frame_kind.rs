use serde::Deserialize;

#[derive(Deserialize)]
pub(crate) struct FrameKind<'a> {
    pub(crate) kind: &'a str,
}

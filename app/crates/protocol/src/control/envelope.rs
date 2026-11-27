use serde::Serialize;

#[derive(Serialize)]
pub(crate) struct Envelope<'a, T: Serialize> {
    pub(crate) sdk: u32,
    #[serde(flatten)]
    pub(crate) frame: &'a T,
}

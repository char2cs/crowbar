pub(crate) mod centring;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Reveal {
    Hidden,
    Find,
    Replace,
}

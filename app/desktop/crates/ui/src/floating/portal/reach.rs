#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Reach {
    Here,
    Window,
    Cover,
    Lifted,
}

/// Which open or save of a view's document is the latest: a disk read begun under an older mark
/// lands on text it no longer describes, so it is dropped, as develop's `documentSaveMark`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct SaveMark(pub(crate) u64);

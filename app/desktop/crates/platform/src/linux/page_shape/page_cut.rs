use crate::linux::float::Card;
use crate::linux::page_shape::rounded::Rounded;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PageCut {
    Trimmed {
        hole: Option<Card>,
        round: Option<Rounded>,
    },
    Everything,
}

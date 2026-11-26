//! What the Files panel reports while one of its rows is pressed and dragged.

use gpui::{Pixels, Point};
use sidebar::{DropSpot, FileMove};

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum FileGesture {
    Press(FileMove, Point<Pixels>),
    Over(DropSpot, Point<Pixels>),
    Left(DropSpot),
    Roam(Point<Pixels>),
    Away,
    Edge(f32),
    Dropped,
}

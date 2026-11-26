//! One overlay open in a window: its kind, the cards it painted and its click-away handler.

use std::rc::Rc;

use gpui::{App, Bounds, Pixels, Window};

use crate::floating::portal::float::Float;

pub(crate) type Away = Rc<dyn Fn(&mut Window, &mut App)>;

pub(crate) struct Entry {
    pub(crate) float: Float,
    pub(crate) cards: Vec<Bounds<Pixels>>,
    pub(crate) away: Option<Away>,
}

use gpui::ElementId;

use crate::floating::portal::cover::Away;
use crate::floating::portal::float::Float;

pub(crate) struct Opening(
    pub(crate) ElementId,
    pub(crate) Float,
    pub(crate) Option<Away>,
);

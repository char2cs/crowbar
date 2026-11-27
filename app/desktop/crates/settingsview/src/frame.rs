mod element_id;
mod field;
mod fields;
mod icon_stem;
mod row_layout;
pub(crate) mod spacing;

pub(crate) use element_id::element_id;
pub(crate) use field::Field;
pub(crate) use fields::Fields;
pub(crate) use icon_stem::icon_named;
pub(crate) use row_layout::RowLayout;

use gpui::{FocusHandle, ScrollHandle};
use settings::RecordBackend;

use crate::intent::Sink;
use crate::lift::Lift;

#[derive(Clone, Copy)]
pub(crate) struct Frame<'a> {
    pub(crate) state: &'a settings::State,
    pub(crate) sink: &'a Sink,
    pub(crate) fields: &'a Fields,
    pub(crate) records: &'a dyn RecordBackend,
    pub(crate) focus: &'a FocusHandle,
    pub(crate) scroll: &'a ScrollHandle,
    pub(crate) lift: Option<&'a Lift>,
    pub(crate) rows: RowLayout,
}

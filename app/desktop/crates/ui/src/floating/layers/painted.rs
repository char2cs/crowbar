use crate::floating::layers::float_region::FloatRegion;

#[derive(Debug, Clone, Default)]
pub(crate) struct Painted {
    pub(crate) seen: Vec<FloatRegion>,
    pub(crate) hit: Vec<FloatRegion>,
}

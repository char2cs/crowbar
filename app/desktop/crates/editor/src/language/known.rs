#[derive(Debug, Clone, Copy)]
pub(super) struct Known {
    pub(super) id: &'static str,
    pub(super) name: &'static str,
    pub(super) extensions: &'static [&'static str],
    pub(super) file_names: &'static [&'static str],
}

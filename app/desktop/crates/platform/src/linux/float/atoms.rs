//! The atoms a float placer names to the X server.

#[derive(Debug, Clone, Copy)]
pub(crate) struct Atoms {
    pub(crate) compositor: u32,
    pub(crate) state: u32,
    pub(crate) skip_taskbar: u32,
    pub(crate) skip_pager: u32,
}

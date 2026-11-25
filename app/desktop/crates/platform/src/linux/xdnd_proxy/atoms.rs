//! The atoms of the XDND protocol the proxy reads, writes and sends.

x11rb::atom_manager! {
    pub(crate) Atoms: AtomsCookie {
        XdndAware,
        XdndProxy,
        XdndTypeList,
        XdndEnter,
        XdndPosition,
        XdndStatus,
        XdndLeave,
        XdndDrop,
        XdndFinished,
    }
}

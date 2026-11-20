//! Whether a pane's page is listening for the shell's frames.

use protocol::PaneId;

use crate::pane_host::PaneHost;
use crate::pane_host::lifecycle::pane_runtime::PaneRuntime;

impl PaneHost {
    /// Whether `pane`'s page registered and was sent its `mount`, so a frame sent now is heard.
    #[must_use]
    pub fn is_mounted(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes.get(pane).is_some_and(PaneRuntime::is_mounted)
    }
}

#[cfg(test)]
mod tests {
    use protocol::PaneId;

    use crate::test_support::{fixture, instance, register};

    #[test]
    fn a_page_is_mounted_once_it_registers_and_not_before() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
        f.sync(&panes, &["p1", "p2"], Some("p1"));
        register(&mut f, "p1", "core.chat");
        let mounted = |id: &str| f.host.is_mounted(&PaneId::new(id).unwrap());
        assert_eq!(
            [mounted("p1"), mounted("p2"), mounted("none")],
            [true, false, false]
        );
    }
}

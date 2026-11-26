use protocol::{PairedHost, ShellFrame};

use crate::error::Result;
use crate::pane_host::PaneHost;

impl PaneHost {
    /// Replaces the hosts the shell paired and pushes the list to mounted panes; a pane mounted
    /// later gets it on mount. An unchanged list sends nothing.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
    pub fn set_paired_hosts(
        &mut self,
        hosts: Vec<PairedHost>,
    ) -> Result<()> {
        if self.paired_hosts.as_ref() == Some(&hosts) {
            return Ok(());
        }
        let frame = ShellFrame::Hosts {
            hosts: hosts.clone(),
        };
        self.paired_hosts = Some(hosts);
        self.panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame))
    }

    pub(crate) fn paired_hosts_context(&self) -> Option<ShellFrame> {
        let hosts = self.paired_hosts.clone()?;
        Some(ShellFrame::Hosts { hosts })
    }
}

#[cfg(test)]
mod tests {
    use protocol::{DisplayText, HostId, PairedHost, ShellFrame};

    use crate::test_support::*;

    fn host(id: &str) -> PairedHost {
        PairedHost {
            id: HostId::new(id).unwrap(),
            name: DisplayText::new(id).unwrap(),
            endpoint: DisplayText::new("127.0.0.1:4747").unwrap(),
        }
    }

    fn sent(f: &Fixture) -> Vec<Vec<PairedHost>> {
        frames_sent(&f.mock)
            .into_iter()
            .filter_map(|frame| match frame {
                ShellFrame::Hosts { hosts } => Some(hosts),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn a_pane_gets_the_paired_hosts_on_mount_and_once_per_change() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        f.host.set_paired_hosts(vec![host("h1")]).unwrap();
        assert!(sent(&f).is_empty(), "not mounted yet");
        register(&mut f, "p1", "core.chat");
        f.host.set_paired_hosts(vec![host("h1")]).unwrap();
        f.host.set_paired_hosts(Vec::new()).unwrap();
        assert_eq!(
            sent(&f),
            vec![vec![host("h1")], Vec::new()],
            "on mount, then once per change"
        );
    }
}

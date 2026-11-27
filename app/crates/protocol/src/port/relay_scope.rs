use crate::identity::HostId;
use crate::identity::PaneId;
use crate::identity::PaneOrigin;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RelayScope {
    host: HostId,
    pane: PaneId,
    origin: PaneOrigin,
}

impl RelayScope {
    #[must_use]
    pub fn new(
        host: HostId,
        pane: PaneId,
        origin: PaneOrigin,
    ) -> Self {
        Self { host, pane, origin }
    }

    #[must_use]
    pub fn host(&self) -> &HostId {
        &self.host
    }

    #[must_use]
    pub fn pane(&self) -> &PaneId {
        &self.pane
    }

    #[must_use]
    pub fn origin(&self) -> &PaneOrigin {
        &self.origin
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn getters_return_what_was_built() {
        let scope = RelayScope::new(
            HostId::new("host-1").unwrap(),
            PaneId::new("p1").unwrap(),
            PaneOrigin::Core,
        );
        assert_eq!(scope.host().as_str(), "host-1", "host");
        assert_eq!(scope.pane().as_str(), "p1", "pane");
        assert_eq!(scope.origin(), &PaneOrigin::Core, "origin");
    }
}

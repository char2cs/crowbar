use protocol::{PaneId, RelayEndpoint, RelayScope};

use crate::error::Error;

pub trait StreamAccess: Send + Sync {
    fn issue(
        &self,
        scope: &RelayScope,
    ) -> Result<RelayEndpoint, Error>;

    fn revoke_pane(
        &self,
        pane: &PaneId,
    );
}

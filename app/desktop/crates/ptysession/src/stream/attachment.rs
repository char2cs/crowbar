use protocol::RelayScope;

use crate::ends::Ends;

#[derive(Debug, Clone)]
pub(crate) struct Attachment {
    pub(crate) scope: RelayScope,
    pub(crate) ends: Ends,
}

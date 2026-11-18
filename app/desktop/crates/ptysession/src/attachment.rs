//! The relay scope and the view's ends one connected session runs over.

use protocol::RelayScope;

use crate::ends::Ends;

#[derive(Debug, Clone)]
pub(crate) struct Attachment {
    pub(crate) scope: RelayScope,
    pub(crate) ends: Ends,
}

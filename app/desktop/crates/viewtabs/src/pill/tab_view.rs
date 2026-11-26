use layout::TabId;

use crate::pill::member_view::MemberView;

/// One tab of the strip: a single chat, or a set of chats that share one pill.
#[derive(Debug, Clone, PartialEq)]
pub struct TabView {
    /// The model tab it stands for.
    pub id: TabId,
    /// The chats of the tab in layout order.
    pub members: Vec<MemberView>,
}

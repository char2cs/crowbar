use layout::TabId;

use crate::pill::member_view::MemberView;

#[derive(Debug, Clone, PartialEq)]
pub struct TabView {
    pub id: TabId,
    pub members: Vec<MemberView>,
}

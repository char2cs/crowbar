use crate::identity::pane_type_id::PaneTypeId;

impl PaneTypeId {
    /// The chat pane's type. A chat is the one pane of its group that is not an editor tab.
    pub const CHAT: &'static str = "core.chat";

    /// Whether this is the chat pane's type, [`PaneTypeId::CHAT`].
    #[must_use]
    pub fn is_chat(&self) -> bool {
        self.as_str() == Self::CHAT
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_chat_type_is_a_chat() {
        assert_eq!(PaneTypeId::CHAT, "core.chat", "the chat type's text");
        let cases = [
            ("core.chat", true),
            ("core.terminal", false),
            ("ext.git.chat", false),
        ];
        for (text, chat) in cases {
            assert_eq!(PaneTypeId::new(text).unwrap().is_chat(), chat, "{text}");
        }
    }
}

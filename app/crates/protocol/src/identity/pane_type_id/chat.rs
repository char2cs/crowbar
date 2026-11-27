use crate::identity::pane_type_id::PaneTypeId;

impl PaneTypeId {
    pub const CHAT: &'static str = "core.chat";

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

use crate::keys::shortcut::Shortcut;
use crate::values::Key;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ShortcutGroup {
    pub(crate) title: String,
    pub(crate) overrides: Key,
    pub(crate) shortcuts: Vec<Shortcut>,
    pub(crate) preset: Option<Key>,
}

impl ShortcutGroup {
    #[must_use]
    pub fn title(&self) -> &str {
        &self.title
    }

    #[must_use]
    pub fn shortcuts(&self) -> &[Shortcut] {
        &self.shortcuts
    }

    pub(crate) fn new(
        title: impl Into<String>,
        overrides: impl Into<Key>,
    ) -> Self {
        Self {
            title: title.into(),
            overrides: overrides.into(),
            shortcuts: Vec::new(),
            preset: None,
        }
    }

    #[must_use]
    pub(crate) fn preset_from(
        mut self,
        key: impl Into<Key>,
    ) -> Self {
        self.preset = Some(key.into());
        self
    }

    #[must_use]
    pub(crate) fn shortcut(
        mut self,
        shortcut: Shortcut,
    ) -> Self {
        self.shortcuts.push(shortcut);
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn shortcuts_keep_their_order() {
        let group = ShortcutGroup::new("Tabs", "overrides")
            .shortcut(Shortcut::new("a", "A", "mod+a"))
            .shortcut(Shortcut::new("b", "B", ""));
        let commands: Vec<&str> = group.shortcuts.iter().map(Shortcut::command).collect();
        assert_eq!(commands, ["a", "b"]);
        assert_eq!(group.overrides.as_str(), "overrides");
    }
}

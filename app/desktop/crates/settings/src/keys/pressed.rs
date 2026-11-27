use crate::keys::modifier_key::ModifierKey;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Pressed {
    pub(crate) key: String,
    pub(crate) held: Vec<ModifierKey>,
}

impl Pressed {
    pub fn new(
        key: impl Into<String>,
        held: &[ModifierKey],
    ) -> Self {
        Self {
            key: key.into().to_lowercase(),
            held: held.to_vec(),
        }
    }

    pub(crate) fn has(
        &self,
        modifier: ModifierKey,
    ) -> bool {
        self.held.contains(&modifier)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn keys_are_lowercased_and_modifiers_queryable() {
        let pressed = Pressed::new("ArrowLeft", &[ModifierKey::Alt]);
        assert_eq!(pressed.key, "arrowleft");
        assert!(pressed.has(ModifierKey::Alt));
        assert!(!pressed.has(ModifierKey::Shift));
    }
}

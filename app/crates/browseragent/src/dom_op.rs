/// An operation the page's own script carries out.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DomOp {
    /// Reads the page as a tree of roles, names and refs.
    Snapshot,
    /// Clicks the element with a ref.
    Click,
    /// Types into the field with a ref.
    Type,
    /// Presses a key on the focused element.
    PressKey,
    /// Scrolls the page or brings an element into view.
    Scroll,
    /// Waits for the page to load, a selector or a text.
    Wait,
    /// Finds the elements a pasted pointer names.
    Resolve,
}

impl DomOp {
    pub(crate) fn of(op: &str) -> Option<Self> {
        match op {
            "snapshot" => Some(Self::Snapshot),
            "click" => Some(Self::Click),
            "type" => Some(Self::Type),
            "press_key" => Some(Self::PressKey),
            "scroll" => Some(Self::Scroll),
            "wait" => Some(Self::Wait),
            "resolve" => Some(Self::Resolve),
            _ => None,
        }
    }

    /// The name the page script knows the operation by.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::Snapshot => "snapshot",
            Self::Click => "click",
            Self::Type => "type",
            Self::PressKey => "press_key",
            Self::Scroll => "scroll",
            Self::Wait => "wait",
            Self::Resolve => "resolve",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_operation_is_known_by_the_name_the_daemon_sends() {
        for name in [
            "snapshot",
            "click",
            "type",
            "press_key",
            "scroll",
            "wait",
            "resolve",
        ] {
            assert_eq!(DomOp::of(name).map(DomOp::name), Some(name));
        }
        assert_eq!(DomOp::of("eval"), None);
    }
}

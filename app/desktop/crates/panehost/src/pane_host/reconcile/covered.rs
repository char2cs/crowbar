use webview::Bounds;

/// What the shell's own overlays cover, so that no web view draws over them. Covered panes keep
/// their stage and lifecycle; only their web view is hidden, and the keys stay with the shell.
#[derive(Debug, Clone, Default, PartialEq)]
pub enum Covered {
    /// Nothing is open over the panes.
    #[default]
    Nothing,
    /// A dialog or a drag is open: every web view is hidden.
    Everything,
    /// Popovers are open: the web views whose rectangle meets one of these, in logical pixels
    /// relative to the window, are hidden.
    Areas(Vec<Bounds>),
}

impl Covered {
    /// Whether a pane at `bounds` is under what is open.
    #[must_use]
    pub fn hides(
        &self,
        bounds: &Bounds,
    ) -> bool {
        match self {
            Self::Nothing => false,
            Self::Everything => true,
            Self::Areas(areas) => areas.iter().any(|area| meet(area, bounds)),
        }
    }
}

pub(crate) fn meet(
    a: &Bounds,
    b: &Bounds,
) -> bool {
    a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height
}

#[cfg(test)]
mod tests {
    use super::*;

    const PANE: Bounds = Bounds {
        x: 100.0,
        y: 40.0,
        width: 200.0,
        height: 300.0,
    };

    const fn area(
        x: f64,
        y: f64,
    ) -> Bounds {
        Bounds {
            x,
            y,
            width: 50.0,
            height: 50.0,
        }
    }

    #[test]
    fn a_pane_is_hidden_by_a_dialog_and_by_a_popover_that_meets_it_only() {
        let cases = [
            ("nothing open", Covered::Nothing, false),
            ("a dialog", Covered::Everything, true),
            (
                "a card over it",
                Covered::Areas(vec![area(120.0, 60.0)]),
                true,
            ),
            (
                "a card beside it",
                Covered::Areas(vec![area(40.0, 60.0)]),
                false,
            ),
            (
                "a card touching its edge",
                Covered::Areas(vec![area(50.0, 60.0)]),
                false,
            ),
        ];
        for (case, covered, hidden) in cases {
            assert_eq!(covered.hides(&PANE), hidden, "{case}");
        }
    }
}

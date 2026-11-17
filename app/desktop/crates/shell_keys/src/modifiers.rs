use crate::modifier::Modifier;

const PRIMARY_BIT: u8 = 1;
const SHIFT_BIT: u8 = 2;
const ALT_BIT: u8 = 4;

/// The modifier keys held down together, in no particular order.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Modifiers(u8);

impl Modifiers {
    /// Whether `modifier` is held.
    #[must_use]
    pub fn contains(
        self,
        modifier: Modifier,
    ) -> bool {
        self.0 & bit(modifier) != 0
    }
}

impl FromIterator<Modifier> for Modifiers {
    fn from_iter<Held: IntoIterator<Item = Modifier>>(held: Held) -> Self {
        Self(
            held.into_iter()
                .fold(0, |bits, modifier| bits | bit(modifier)),
        )
    }
}

fn bit(modifier: Modifier) -> u8 {
    match modifier {
        Modifier::Primary => PRIMARY_BIT,
        Modifier::Shift => SHIFT_BIT,
        Modifier::Alt => ALT_BIT,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_set_holds_exactly_the_modifiers_it_was_built_from() {
        let all = [Modifier::Primary, Modifier::Shift, Modifier::Alt];
        let cases = [
            ("none", vec![], [false, false, false]),
            ("primary", vec![Modifier::Primary], [true, false, false]),
            (
                "shift and alt",
                vec![Modifier::Shift, Modifier::Alt],
                [false, true, true],
            ),
            (
                "repeats count once",
                vec![Modifier::Alt, Modifier::Alt],
                [false, false, true],
            ),
        ];
        for (name, held, expected) in cases {
            let set: Modifiers = held.into_iter().collect();
            let got = all.map(|modifier| set.contains(modifier));
            assert_eq!(got, expected, "{name}");
        }
    }
}

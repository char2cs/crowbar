//! `#rrggbbaa` colours, parsed strictly.
//!
//! ANCHORS.md §3 defines every colour field as `#rrggbbaa` sRGB and §5 defines
//! the comparison: **RGB exact, alpha ±1/255**.
//!
//! Parsing refuses everything else, including the six-digit `#rrggbb` form that
//! appears in the older spec §8.1 sketch. That is deliberate. Accepting six
//! digits means inventing an alpha, and the only defensible invention is `ff` —
//! at which point a genuinely opaque colour and a colour whose alpha the
//! extractor simply failed to emit become indistinguishable. Worse, a parser
//! that fell back to a default on malformed input would make every broken
//! colour compare equal to black, so a real black would look like a pass and a
//! garbage value would look like one too.
//!
//! The rule is therefore: parse `#rrggbbaa`, or return an error that says
//! exactly what was wrong.

use std::fmt;

/// An 8-bit-per-channel sRGB colour with alpha.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Color {
    /// Red, 0–255.
    pub r: u8,
    /// Green, 0–255.
    pub g: u8,
    /// Blue, 0–255.
    pub b: u8,
    /// Alpha, 0–255.
    pub a: u8,
}

/// Why a colour string was rejected.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ParseError {
    /// No leading `#`.
    MissingHash(String),
    /// Not exactly eight hex digits after the `#`.
    WrongLength {
        /// The whole offending string.
        raw: String,
        /// How many characters followed the `#`.
        found: usize,
    },
    /// A character that is not a hex digit.
    NotHex {
        /// The whole offending string.
        raw: String,
        /// The offending character.
        found: char,
    },
}

impl fmt::Display for ParseError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::MissingHash(raw) => write!(
                f,
                "colour `{raw}` does not start with `#`; ANCHORS.md §3 requires `#rrggbbaa`"
            ),
            Self::WrongLength { raw, found } => write!(
                f,
                "colour `{raw}` has {found} characters after the `#`, not 8; ANCHORS.md §3 \
                 requires `#rrggbbaa` and the alpha is not optional — inventing one would \
                 make a missing alpha indistinguishable from an opaque colour"
            ),
            Self::NotHex { raw, found } => {
                write!(
                    f,
                    "colour `{raw}` contains `{found}`, which is not a hex digit"
                )
            }
        }
    }
}

impl std::error::Error for ParseError {}

impl Color {
    /// Parse `#rrggbbaa`.
    ///
    /// Case-insensitive on the hex digits. Nothing else is accepted: no
    /// three-digit shorthand, no six-digit form, no named colours, no
    /// whitespace.
    ///
    /// # Errors
    ///
    /// [`ParseError`] naming exactly which rule the input broke.
    pub fn parse(raw: &str) -> Result<Self, ParseError> {
        let Some(digits) = raw.strip_prefix('#') else {
            return Err(ParseError::MissingHash(raw.to_owned()));
        };
        let count = digits.chars().count();
        if count != 8 {
            return Err(ParseError::WrongLength {
                raw: raw.to_owned(),
                found: count,
            });
        }
        let mut nibbles = [0_u8; 8];
        for (slot, c) in nibbles.iter_mut().zip(digits.chars()) {
            *slot = hex_digit(c, raw)?;
        }
        Ok(Self {
            r: nibbles[0] * 16 + nibbles[1],
            g: nibbles[2] * 16 + nibbles[3],
            b: nibbles[4] * 16 + nibbles[5],
            a: nibbles[6] * 16 + nibbles[7],
        })
    }

    /// The largest absolute difference across `r`, `g` and `b`.
    ///
    /// Used both to decide whether two colours differ in RGB at all (§5 makes
    /// RGB exact, so any non-zero value is a defect) and to rank colour deltas
    /// by how wrong they are.
    #[must_use]
    pub const fn rgb_distance(self, other: Self) -> u8 {
        let dr = self.r.abs_diff(other.r);
        let dg = self.g.abs_diff(other.g);
        let db = self.b.abs_diff(other.b);
        let max_rg = if dr > dg { dr } else { dg };
        if max_rg > db { max_rg } else { db }
    }

    /// Absolute difference in alpha, in 1/255 units.
    #[must_use]
    pub const fn alpha_distance(self, other: Self) -> u8 {
        self.a.abs_diff(other.a)
    }

    /// The channel that differs most, named for an error message, together with
    /// the signed difference `self - other` on that channel.
    ///
    /// Alpha is only ever reported when no RGB channel differs, because §5
    /// makes RGB exact and alpha tolerant: an RGB difference is always the more
    /// important fact.
    #[must_use]
    pub fn worst_channel(self, other: Self) -> (&'static str, i16) {
        // First-wins on a tie, so the answer is stable across runs. `max_by_key`
        // would pick the *last* maximum, which is equally deterministic but
        // reads backwards when r and g are equally wrong.
        let mut worst = ("r", i16::from(self.r) - i16::from(other.r));
        for candidate in [
            ("g", i16::from(self.g) - i16::from(other.g)),
            ("b", i16::from(self.b) - i16::from(other.b)),
        ] {
            if candidate.1.abs() > worst.1.abs() {
                worst = candidate;
            }
        }
        if worst.1 == 0 {
            ("a", i16::from(self.a) - i16::from(other.a))
        } else {
            worst
        }
    }
}

/// One hex digit, or a [`ParseError::NotHex`] naming the offending character.
///
/// Goes through `u8::try_from` so a non-ASCII character is reported as itself
/// rather than as a truncated byte.
fn hex_digit(c: char, raw: &str) -> Result<u8, ParseError> {
    let not_hex = || ParseError::NotHex {
        raw: raw.to_owned(),
        found: c,
    };
    let byte = u8::try_from(c).map_err(|_| not_hex())?;
    match byte {
        b'0'..=b'9' => Ok(byte - b'0'),
        b'a'..=b'f' => Ok(byte - b'a' + 10),
        b'A'..=b'F' => Ok(byte - b'A' + 10),
        _ => Err(not_hex()),
    }
}

impl fmt::Display for Color {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "#{:02x}{:02x}{:02x}{:02x}",
            self.r, self.g, self.b, self.a
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const fn c(r: u8, g: u8, b: u8, a: u8) -> Color {
        Color { r, g, b, a }
    }

    #[test]
    fn parses_the_canonical_form() {
        let cases: &[(&str, Color)] = &[
            ("#00000000", c(0, 0, 0, 0)),
            ("#ffffffff", c(255, 255, 255, 255)),
            ("#c8ccd4ff", c(0xc8, 0xcc, 0xd4, 0xff)),
            ("#C8CCD4FF", c(0xc8, 0xcc, 0xd4, 0xff)),
            ("#1e2228aa", c(0x1e, 0x22, 0x28, 0xaa)),
        ];
        for (raw, want) in cases {
            assert_eq!(Color::parse(raw).as_ref(), Ok(want), "input {raw}");
        }
    }

    #[test]
    fn round_trips_through_display() {
        let color = Color::parse("#1e2228aa").expect("valid");
        assert_eq!(color.to_string(), "#1e2228aa");
        assert_eq!(Color::parse(&color.to_string()), Ok(color));
    }

    #[test]
    fn rejects_everything_that_is_not_rrggbbaa() {
        assert_eq!(
            Color::parse("c8ccd4ff"),
            Err(ParseError::MissingHash("c8ccd4ff".to_owned()))
        );
        // The six-digit form from the older §8.1 sketch is rejected on purpose:
        // there is no defensible alpha to invent.
        assert_eq!(
            Color::parse("#c8ccd4"),
            Err(ParseError::WrongLength {
                raw: "#c8ccd4".to_owned(),
                found: 6
            })
        );
        assert_eq!(
            Color::parse("#fff"),
            Err(ParseError::WrongLength {
                raw: "#fff".to_owned(),
                found: 3
            })
        );
        assert_eq!(
            Color::parse("#c8ccd4ffff"),
            Err(ParseError::WrongLength {
                raw: "#c8ccd4ffff".to_owned(),
                found: 10
            })
        );
        assert_eq!(
            Color::parse("#"),
            Err(ParseError::WrongLength {
                raw: "#".to_owned(),
                found: 0
            })
        );
        assert_eq!(
            Color::parse("#c8ccd4fz"),
            Err(ParseError::NotHex {
                raw: "#c8ccd4fz".to_owned(),
                found: 'z'
            })
        );
        assert_eq!(
            Color::parse("#c8ccd4f😀"),
            Err(ParseError::NotHex {
                raw: "#c8ccd4f😀".to_owned(),
                found: '😀'
            })
        );
        assert_eq!(
            Color::parse(""),
            Err(ParseError::MissingHash(String::new()))
        );
    }

    #[test]
    fn every_parse_error_renders_and_names_the_rule() {
        let errors = [
            ParseError::MissingHash("x".to_owned()),
            ParseError::WrongLength {
                raw: "#fff".to_owned(),
                found: 3,
            },
            ParseError::NotHex {
                raw: "#zzzzzzzz".to_owned(),
                found: 'z',
            },
        ];
        for e in errors {
            let rendered = e.to_string();
            assert!(!rendered.is_empty(), "{e:?} rendered empty");
        }
    }

    #[test]
    fn distances_measure_the_right_thing() {
        let base = c(0x10, 0x20, 0x30, 0x40);
        assert_eq!(base.rgb_distance(base), 0);
        assert_eq!(base.alpha_distance(base), 0);
        assert_eq!(base.rgb_distance(c(0x11, 0x20, 0x30, 0x40)), 1);
        assert_eq!(base.rgb_distance(c(0x10, 0x25, 0x30, 0x40)), 5);
        assert_eq!(base.rgb_distance(c(0x10, 0x20, 0x39, 0x40)), 9);
        // Alpha never contributes to the RGB distance.
        assert_eq!(base.rgb_distance(c(0x10, 0x20, 0x30, 0xff)), 0);
        assert_eq!(base.alpha_distance(c(0x10, 0x20, 0x30, 0x41)), 1);
    }

    #[test]
    fn worst_channel_prefers_rgb_and_keeps_the_sign() {
        let base = c(0x10, 0x20, 0x30, 0x40);
        assert_eq!(base.worst_channel(c(0x08, 0x20, 0x30, 0x40)), ("r", 8));
        assert_eq!(base.worst_channel(c(0x10, 0x30, 0x30, 0x40)), ("g", -16));
        assert_eq!(base.worst_channel(c(0x10, 0x20, 0x00, 0x40)), ("b", 48));
        // Only when RGB agrees does alpha get reported.
        assert_eq!(base.worst_channel(c(0x10, 0x20, 0x30, 0x3c)), ("a", 4));
        assert_eq!(base.worst_channel(base), ("a", 0));
        // A tie between two channels still names one of them, deterministically.
        assert_eq!(base.worst_channel(c(0x08, 0x18, 0x30, 0x40)), ("r", 8));
    }
}

const PAINT_ATTRIBUTES: [&str; 2] = ["fill", "stroke"];
const CURRENT_COLOR: &str = "currentColor";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct MaterialIcon {
    pub(crate) name: &'static str,
    pub(crate) svg: &'static str,
}

impl MaterialIcon {
    #[must_use]
    pub fn name(self) -> &'static str {
        self.name
    }

    #[must_use]
    pub fn colored(self) -> &'static str {
        self.svg
    }

    #[must_use]
    pub fn monochrome(self) -> String {
        PAINT_ATTRIBUTES
            .iter()
            .fold(self.svg.to_owned(), |svg, attribute| {
                recolored(&svg, attribute)
            })
    }
}

fn recolored(
    svg: &str,
    attribute: &str,
) -> String {
    let opening = format!("{attribute}=\"");
    let mut pieces = svg.split(opening.as_str());
    let head = pieces.next().unwrap_or_default().to_owned();
    let painted = pieces.map(|piece| repainted(piece, &opening));
    std::iter::once(head).chain(painted).collect()
}

fn repainted(
    piece: &str,
    opening: &str,
) -> String {
    let after = piece.split_once('"').map_or(piece, |(_, after)| after);
    format!("{opening}{CURRENT_COLOR}\"{after}")
}

#[cfg(test)]
mod tests {
    use super::*;

    const ART: &str = r##"<svg><path fill="#e44d26" stroke="#fff"/><path fill="none"/></svg>"##;

    #[test]
    fn the_one_colour_artwork_paints_every_fill_and_stroke_in_the_current_colour() {
        let icon = MaterialIcon {
            name: "html",
            svg: ART,
        };
        assert_eq!(icon.colored(), ART);
        assert_eq!(
            icon.monochrome(),
            r#"<svg><path fill="currentColor" stroke="currentColor"/><path fill="currentColor"/></svg>"#
        );
    }
}

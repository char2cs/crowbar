//! The font families of this computer, and the font and theme selects they fill.

use settings::Choice;

/// Every family, and the ones whose letters all share one advance width.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct SystemFonts {
    /// Every installed family.
    pub all: Vec<String>,
    /// The monospace families among them.
    pub monospace: Vec<String>,
}

const BUNDLED_UI: [&str; 3] = ["CalSansUI", "Geist Variable", "Instrument Serif"];
const BUNDLED_MONO: [&str; 1] = ["Geist Mono Variable"];
const THEMES: [(&str, &str); 2] = [("crowbar", "Crowbar"), ("frosted", "Frosted")];

pub(crate) fn themes() -> Vec<Choice> {
    THEMES
        .into_iter()
        .map(|(value, label)| Choice::new(value, label))
        .collect()
}

pub(crate) fn fonts(system: &SystemFonts) -> Vec<Choice> {
    listed(BUNDLED_UI.iter().chain(&BUNDLED_MONO), &system.all)
}

pub(crate) fn monospace(system: &SystemFonts) -> Vec<Choice> {
    listed(BUNDLED_MONO.iter(), &system.monospace)
}

fn listed<'a>(
    bundled: impl Iterator<Item = &'a &'a str>,
    system: &[String],
) -> Vec<Choice> {
    let mut names: Vec<&str> = bundled.copied().collect();
    let extra = system
        .iter()
        .map(String::as_str)
        .filter(|name| !names.contains(name));
    let extra: Vec<&str> = extra.collect();
    names.extend(extra);
    names
        .into_iter()
        .map(|name| Choice::new(name, name))
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn bundled_fonts_come_first_and_a_system_family_is_listed_once() {
        let system = SystemFonts {
            all: vec!["Geist Variable".into(), "DejaVu Sans".into()],
            monospace: vec!["DejaVu Sans Mono".into()],
        };
        let values: Vec<String> = fonts(&system)
            .iter()
            .map(|choice| choice.value().to_owned())
            .collect();
        assert_eq!(
            values,
            [
                "CalSansUI",
                "Geist Variable",
                "Instrument Serif",
                "Geist Mono Variable",
                "DejaVu Sans"
            ]
        );
        assert_eq!(monospace(&system).len(), 2);
        assert_eq!(themes().len(), 2);
    }
}

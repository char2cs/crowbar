use crate::foundation::icon::material_icon::MaterialIcon;

const FIELD: char = '\t';
const LIST: char = ',';

pub(crate) struct MaterialEntry {
    pub(crate) icon: MaterialIcon,
    files: Vec<&'static str>,
    extensions: Vec<&'static str>,
}

impl MaterialEntry {
    pub(crate) fn parse(line: &'static str) -> Option<Self> {
        let mut fields = line.split(FIELD);
        let name = fields.next()?;
        let files = list(fields.next()?);
        let extensions = list(fields.next()?);
        let svg = fields.next()?;
        Some(Self {
            icon: MaterialIcon { name, svg },
            files,
            extensions,
        })
    }

    pub(crate) fn names(
        &self,
        lowered: &str,
    ) -> bool {
        self.files.contains(&lowered)
    }

    pub(crate) fn extension_of(
        &self,
        lowered: &str,
    ) -> Option<usize> {
        self.extensions
            .iter()
            .filter(|extension| ends_with_extension(lowered, extension))
            .map(|extension| extension.len())
            .max()
    }
}

fn ends_with_extension(
    lowered: &str,
    extension: &str,
) -> bool {
    lowered
        .strip_suffix(extension)
        .is_some_and(|stem| stem.ends_with('.'))
}

fn list(field: &'static str) -> Vec<&'static str> {
    field.split(LIST).filter(|item| !item.is_empty()).collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_line_names_its_files_and_measures_its_longest_extension() {
        let entry = MaterialEntry::parse("ts\ttsconfig.json\tts,d.ts\t<svg/>");
        let found = entry.map(|entry| {
            (
                entry.icon.name,
                entry.names("tsconfig.json"),
                entry.extension_of("a.d.ts"),
                entry.extension_of("a.rs"),
            )
        });
        assert_eq!(found, Some(("ts", true, Some(4), None)));
        assert!(MaterialEntry::parse("half\tline").is_none());
    }
}

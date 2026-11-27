use editor::grammar::QueryPatch;
use gpui_kit::component::highlighter::LanguageRegistry;

pub(crate) fn register() -> Result<(), editor::Error> {
    editor::grammar::PATCHES.iter().try_for_each(rewrite)
}

fn rewrite(patch: &QueryPatch) -> Result<(), editor::Error> {
    let registry = LanguageRegistry::singleton();
    let Some(mut config) = registry.language(patch.language) else {
        return Ok(());
    };
    let highlights = patch.apply(&config.highlights)?;
    if highlights == config.highlights.as_ref() {
        return Ok(());
    }
    config.highlights = highlights.into();
    registry.register(patch.language, &config);
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_rewrite_still_finds_the_query_text_it_replaces() {
        let registered = register();
        assert!(registered.is_ok(), "{registered:?}");
        let registry = LanguageRegistry::singleton();
        let typescript = registry
            .language("typescript")
            .map(|config| config.highlights);
        assert!(typescript.is_some_and(|query| query.contains("(predefined_type) @keyword")));
    }
}

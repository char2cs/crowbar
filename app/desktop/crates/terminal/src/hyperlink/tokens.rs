use std::ops::Range;

use crate::hyperlink::link_target::LinkTarget;
use crate::hyperlink::link_token::LinkToken;
use crate::hyperlink::scan::PathMatch;
use crate::hyperlink::scan::address_runs;
use crate::hyperlink::scan::find_paths;
use crate::hyperlink::scan::find_urls;

pub(crate) fn tokenize(text: &str) -> Vec<LinkToken> {
    let runs = address_runs(text);
    let urls = find_urls(text)
        .into_iter()
        .filter_map(|span| url_token(text, span));
    let files = find_paths(text)
        .into_iter()
        .filter_map(|found| file_token(text, &found, &runs));
    urls.chain(files).collect()
}

fn url_token(
    text: &str,
    span: Range<usize>,
) -> Option<LinkToken> {
    let address = text.get(span.clone())?.to_owned();
    Some(LinkToken {
        target: LinkTarget::Url(address),
        start: span.start,
        end: span.end,
    })
}

fn file_token(
    text: &str,
    found: &PathMatch,
    runs: &[Range<usize>],
) -> Option<LinkToken> {
    let inside_address = runs.iter().any(|run| run.contains(&found.span.start));
    if inside_address {
        return None;
    }
    let path = text.get(found.span.start..found.path_end)?.to_owned();
    let target = LinkTarget::File {
        path,
        line: found.line,
    };
    Some(LinkToken {
        target,
        start: found.span.start,
        end: found.span.end,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn urls_and_files_are_both_found_and_addresses_hide_their_paths() {
        let tokens = tokenize("see https://x.dev/a/b and src/lib.rs:3 or ftp://h/a/b");
        let targets: Vec<LinkTarget> = tokens.into_iter().map(|token| token.target).collect();
        assert_eq!(
            targets,
            [
                LinkTarget::Url("https://x.dev/a/b".into()),
                LinkTarget::File {
                    path: "src/lib.rs".into(),
                    line: Some(3)
                },
            ]
        );
    }
}

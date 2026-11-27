use settings::{Line, Record, Tone};

use crate::store::settings_core::agents::descriptor_finding::DescriptorFinding;
use crate::store::settings_core::agents::descriptor_report::DescriptorReport;

const SHIPPED: &str = "shipped";
const OVERRIDE_REFUSED: &str = "Override refused \u{2014} using shipped";
const BLOCKED: &str = "Blocked";
const OK: &str = "OK";

pub(crate) fn of(reports: &[DescriptorReport]) -> Vec<Record> {
    reports.iter().map(record).collect()
}

fn record(report: &DescriptorReport) -> Record {
    let source = Some(report.source.as_str())
        .filter(|name| !name.is_empty())
        .unwrap_or(SHIPPED);
    let record = Record::new(report.id.as_str(), report.id.as_str())
        .aside(Line::plain(source))
        .badge(badge(report));
    report.findings.iter().fold(record, |held, finding| {
        let held = held.line(Line::plain(finding_text(finding)));
        match finding.hint.as_str() {
            "" => held,
            hint => held.line(Line::plain(format!("Fix: {hint}"))),
        }
    })
}

fn badge(report: &DescriptorReport) -> Line {
    if report.fell_back.is_on() {
        return Line::plain(OVERRIDE_REFUSED).toned(Tone::Warning);
    }
    if report.errors() > 0 {
        return Line::plain(BLOCKED).toned(Tone::Error);
    }
    match report.warnings() {
        0 => Line::plain(OK).toned(Tone::Success),
        1 => Line::plain("1 warning").toned(Tone::Warning),
        many => Line::plain(format!("{many} warnings")).toned(Tone::Warning),
    }
}

fn finding_text(finding: &DescriptorFinding) -> String {
    let place = Some(finding.path.as_str())
        .filter(|path| !path.is_empty())
        .unwrap_or("(document)");
    let at = Some(finding.line)
        .filter(|line| *line > 0)
        .map_or_else(|| place.to_owned(), |line| format!("{place} line {line}"));
    format!(
        "{} {} at {at}: {}",
        finding.severity, finding.rule, finding.message
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    fn report(json: &str) -> Record {
        let report: DescriptorReport = serde_json::from_str(json).unwrap();
        record(&report)
    }

    const ERROR: &str = r#"{"rule":"r","severity":"error","path":"a.b","line":3,"message":"m"}"#;
    const WARNING: &str =
        r#"{"rule":"r","severity":"warning","path":"a.b","line":3,"message":"m"}"#;

    fn json(
        extra: &str,
        findings: &[&str],
    ) -> String {
        format!(
            r#"{{"id":"claude"{extra},"findings":[{}]}}"#,
            findings.join(",")
        )
    }

    #[test]
    fn each_descriptor_reads_id_source_and_one_badge_with_its_findings_under_it() {
        let cases = [
            (json("", &[]), SHIPPED, ("OK", Tone::Success)),
            (
                json(r#","source":"/o.yaml""#, &[ERROR]),
                "/o.yaml",
                ("Blocked", Tone::Error),
            ),
            (
                json("", &[WARNING, WARNING]),
                SHIPPED,
                ("2 warnings", Tone::Warning),
            ),
            (
                json(r#","fellBack":true"#, &[ERROR]),
                SHIPPED,
                (OVERRIDE_REFUSED, Tone::Warning),
            ),
        ];
        for (text, source, (badge, tone)) in cases {
            let held = report(&text);
            let findings = text.matches("\"rule\"").count();
            assert_eq!(held.aside_line(), Some(&Line::plain(source)), "{text}");
            assert_eq!(
                held.badge_line(),
                Some(&Line::plain(badge).toned(tone)),
                "{text}"
            );
            assert_eq!(held.lines().len(), findings, "{text}");
        }
    }
}

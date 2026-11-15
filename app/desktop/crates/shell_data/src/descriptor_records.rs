//! The Descriptor checks list: one record per descriptor, its findings as lines.

use settings_ui::{Line, Record, Tone};

use crate::descriptor_finding::DescriptorFinding;
use crate::descriptor_report::DescriptorReport;

const SHIPPED: &str = "shipped";
const OVERRIDE_REFUSED: &str = "Override refused: the shipped descriptor runs";

pub(crate) fn of(reports: &[DescriptorReport]) -> Vec<Record> {
    reports.iter().map(record).collect()
}

fn record(report: &DescriptorReport) -> Record {
    let source = Some(report.source.as_str())
        .filter(|name| !name.is_empty())
        .unwrap_or(SHIPPED);
    let record = Record::new(report.id.as_str(), report.id.as_str())
        .status(tone(report))
        .line(Line::plain(source).mono());
    let record = fallback(record, report);
    report.findings.iter().fold(record, |held, finding| {
        held.line(Line::plain(finding_text(finding)))
    })
}

fn fallback(
    record: Record,
    report: &DescriptorReport,
) -> Record {
    if report.fell_back.is_on() {
        return record.line(Line::plain(OVERRIDE_REFUSED));
    }
    record
}

fn tone(report: &DescriptorReport) -> Tone {
    let only_warnings = report.warnings() > 0 && report.errors() == 0;
    if report.fell_back.is_on() || only_warnings {
        return Tone::Warning;
    }
    if report.errors() > 0 {
        return Tone::Error;
    }
    Tone::Success
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

    #[test]
    fn a_clean_descriptor_is_ok_and_an_error_blocks() {
        let clean = report(r#"{"id":"claude"}"#);
        let expected = Record::new("claude", "claude")
            .status(Tone::Success)
            .line(Line::plain(SHIPPED).mono());
        assert_eq!(clean, expected);
        let blocked = report(
            r#"{"id":"x","source":"/o.yaml","findings":[{"rule":"r","severity":"error","path":"a.b","line":3,"message":"m"}]}"#,
        );
        let expected = Record::new("x", "x")
            .status(Tone::Error)
            .line(Line::plain("/o.yaml").mono())
            .line(Line::plain("error r at a.b line 3: m"));
        assert_eq!(blocked, expected);
    }
}

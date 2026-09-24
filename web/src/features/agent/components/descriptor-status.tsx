import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { getDescriptorReports } from '@/features/agent/api/agent-api'
import type { DescriptorReport } from '@/features/agent/api/agent-api'

type Load =
  { state: 'loading' } | { state: 'failed' } | { state: 'ready'; reports: DescriptorReport[] }

/**
 * Each provider descriptor's static validation, as the daemon runs it at boot:
 * an error blocks the provider (it is never enabled), a warning is advice.
 */
export function DescriptorStatus() {
  const [load, setLoad] = useState<Load>({ state: 'loading' })

  useEffect(() => {
    let cancelled = false
    void getDescriptorReports()
      .then((reports) => {
        if (!cancelled) setLoad({ state: 'ready', reports })
      })
      .catch(() => {
        if (!cancelled) setLoad({ state: 'failed' })
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <div data-testid="descriptor-status" className="space-y-2 px-1 pt-3">
      <h4 className="ui-font ui-text-sm font-medium text-foreground">Descriptor checks</h4>
      {load.state === 'loading' && (
        <p className="ui-font ui-text-sm text-muted-foreground">Checking descriptors…</p>
      )}
      {load.state === 'failed' && (
        <p className="ui-font ui-text-sm text-muted-foreground">
          Crowbar could not check the descriptors — it could not reach the daemon.
        </p>
      )}
      {load.state === 'ready' &&
        load.reports.map((report) => <DescriptorReportRow key={report.id} report={report} />)}
    </div>
  )
}

function DescriptorReportRow({ report }: { report: DescriptorReport }) {
  const errors = report.findings.filter((f) => f.severity === 'error').length
  const warnings = report.findings.length - errors
  return (
    <div data-testid={`descriptor-${report.id}`} className="space-y-1">
      <div className="flex items-center gap-2">
        <span className="ui-font ui-text-sm text-foreground">{report.id}</span>
        <span className="ui-font ui-text-xs truncate text-muted-foreground">
          {report.source ?? 'shipped'}
        </span>
        {errors > 0 ? (
          <Badge variant="error">Blocked</Badge>
        ) : warnings > 0 ? (
          <Badge variant="warning">
            {warnings} {warnings === 1 ? 'warning' : 'warnings'}
          </Badge>
        ) : (
          <Badge variant="success">OK</Badge>
        )}
      </div>
      {report.findings.length > 0 && (
        <ul className="space-y-1 pl-3">
          {report.findings.map((f) => (
            <li
              key={`${f.rule}:${f.path}:${f.line}`}
              className="ui-font ui-text-xs text-muted-foreground"
            >
              <span className={f.severity === 'error' ? 'text-destructive-foreground' : ''}>
                {f.severity}
              </span>{' '}
              <code>{f.rule}</code> at <code>{f.path || '(document)'}</code>
              {f.line > 0 && ` line ${f.line}`}: {f.message}
              {f.hint && <span className="block pl-3">Fix: {f.hint}</span>}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

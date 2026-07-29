import { useEffect, useMemo, useState } from 'react'
import { TriangleAlert, Wand2 } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Select,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import {
  buildExpression,
  DAY_NAMES,
  nextRuns,
  parseCron,
  RUN_COUNT,
  untilText,
  type BuildFrequency,
  type BuildSpec,
  type CronField,
} from './cron'

const NEVER_RUNS =
  'This expression is valid but never runs. Check the day and month together: there is no 30 February, for example.'

const FIELD_LABELS: Record<string, string> = {
  minute: 'Minute',
  hour: 'Hour',
  dayOfMonth: 'Day of month',
  month: 'Month',
  dayOfWeek: 'Day of week',
}

const FREQUENCIES = [
  { value: 'minutes', label: 'Every N minutes' },
  { value: 'hourly', label: 'Every hour' },
  { value: 'daily', label: 'Every day' },
  { value: 'weekly', label: 'Every week' },
  { value: 'monthly', label: 'Every month' },
]

const EVERY_MINUTES = [1, 2, 5, 10, 15, 20, 30].map((n) => ({ value: String(n), label: String(n) }))
const MINUTES = Array.from({ length: 60 }, (_, n) => ({
  value: String(n),
  label: String(n).padStart(2, '0'),
}))
const HOURS = Array.from({ length: 24 }, (_, n) => ({
  value: String(n),
  label: String(n).padStart(2, '0'),
}))
const WEEKDAYS = DAY_NAMES.map((name, n) => ({ value: String(n), label: name }))
const MONTH_DAYS = Array.from({ length: 28 }, (_, i) => ({
  value: String(i + 1),
  label: String(i + 1),
}))

interface RunRow {
  iso: string
  when: string
  until: string
}

export default function CronExplainerPage() {
  const [text, setText] = useState('30 2 * * 1-5')
  const [now, setNow] = useState(() => new Date())
  const [spec, setSpec] = useState<BuildSpec>({
    frequency: 'daily',
    everyMinutes: 15,
    minute: 0,
    hour: 2,
    weekday: 1,
    day: 1,
  })

  // "From now" would otherwise go stale while the tech reads it.
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 30_000)
    return () => clearInterval(timer)
  }, [])

  const parsed = useMemo(() => parseCron(text), [text])
  const expression = parsed.ok ? parsed.expression : null

  const runs = useMemo<RunRow[]>(() => {
    if (expression === null) return []
    return nextRuns(expression, now, RUN_COUNT).map((date) => ({
      iso: date.toISOString(),
      when: `${DAY_NAMES[date.getDay()]} ${date.toLocaleString()}`,
      until: untilText(now, date),
    }))
  }, [expression, now])

  const fields = useMemo<CronField[]>(
    () =>
      expression === null
        ? []
        : [
            expression.minute,
            expression.hour,
            expression.dayOfMonth,
            expression.month,
            expression.dayOfWeek,
          ],
    [expression],
  )

  const fieldColumns = useMemo<Column<CronField>[]>(
    () => [
      { key: 'label', header: 'Field', width: '10rem', value: (row) => FIELD_LABELS[row.name] },
      { key: 'source', header: 'You typed', width: '8rem' },
      { key: 'english', header: 'Which means' },
      {
        key: 'count',
        header: 'Matches',
        align: 'right',
        width: '6rem',
        value: (row) => row.values.length,
      },
    ],
    [],
  )

  const runColumns = useMemo<Column<RunRow>[]>(
    () => [
      { key: 'when', header: 'Next runs' },
      { key: 'until', header: 'From now', align: 'right', width: '12rem' },
    ],
    [],
  )

  const preview = buildExpression(spec)
  const warnings = expression === null ? [] : [...expression.warnings]
  if (expression !== null && runs.length === 0) warnings.push(NEVER_RUNS)

  return (
    <ToolShell
      title="Cron Explainer"
      description="Read a cron schedule in plain English and see the next five times it will run."
      help={
        <>
          <p>
            A cron line has five fields in this order: minute, hour, day of the month, month, day of
            the week. A "*" means "every one of these". Type the line from the NAS or the backup
            agent and read the sentence underneath, then check the next five run times against the
            job's own log to be sure you are looking at the same schedule.
          </p>
          <p className="mt-2">
            The trap everybody falls into is setting both the day of the month and the day of the
            week. Cron treats those two as "or": 0 3 1 * 1 runs on the 1st of every month and on
            every Monday, not on Mondays that happen to be the 1st. CHIT warns you when an
            expression does that. The other common surprise is a step like */7, which restarts at
            the top of every hour rather than running every 7 minutes forever.
          </p>
          <p className="mt-2">
            The times shown are this computer's local time. If the machine running the job is in
            another time zone, the job runs on its clock, not yours. @reboot is not accepted here
            because it has no schedule: it runs when the machine boots. Expressions with six fields
            (a seconds field first) come from Quartz or Spring, not from crontab, and this tool will
            tell you so rather than guessing.
          </p>
        </>
      }
      actions={
        expression !== null ? (
          <CopyButton value={expression.normalized} label="Copy expression" />
        ) : undefined
      }
    >
      <div className="flex flex-col gap-4">
        <div className="max-w-xl">
          <TextInput
            label="Cron expression"
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder="30 2 * * 1-5"
            spellCheck={false}
            autoComplete="off"
            autoFocus
            className="font-mono"
            hint="Five fields: minute, hour, day of month, month, day of week."
            error={parsed.ok ? undefined : parsed.error}
          />
        </div>

        {expression !== null && (
          <div className="rounded border border-border bg-surface-2 px-3 py-2">
            <p className="text-base text-fg">{expression.english}</p>
            {expression.macro !== '' && (
              <p className="mt-1 text-xs text-fg-muted">
                {expression.macro} is short for {expression.normalized}
              </p>
            )}
          </div>
        )}

        {warnings.map((warning) => (
          <p key={warning} className="flex items-start gap-1.5 text-xs text-warn">
            <TriangleAlert size={14} aria-hidden className="mt-0.5 shrink-0" />
            {warning}
          </p>
        ))}

        {expression !== null && (
          <>
            <ResultsTable
              columns={fieldColumns}
              rows={fields}
              getRowId={(row) => row.name}
              csvName="cron-fields"
              emptyMessage="Nothing to show yet."
            />

            <ResultsTable
              columns={runColumns}
              rows={runs}
              getRowId={(row) => row.iso}
              csvName="cron-next-runs"
              emptyMessage="This expression never runs."
            />
          </>
        )}

        <details className="rounded border border-border bg-surface-2">
          <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg">
            Build an expression
          </summary>
          <div className="flex flex-col gap-2 px-3 pb-3">
            <div className="flex flex-wrap items-end gap-2">
              <Select
                label="Frequency"
                options={FREQUENCIES}
                value={spec.frequency}
                onChange={(event) =>
                  setSpec({ ...spec, frequency: event.target.value as BuildFrequency })
                }
              />
              {spec.frequency === 'minutes' && (
                <Select
                  label="Every N minutes"
                  options={EVERY_MINUTES}
                  value={String(spec.everyMinutes)}
                  onChange={(event) => setSpec({ ...spec, everyMinutes: Number(event.target.value) })}
                />
              )}
              {spec.frequency !== 'minutes' && (
                <Select
                  label="Minute"
                  options={MINUTES}
                  value={String(spec.minute)}
                  onChange={(event) => setSpec({ ...spec, minute: Number(event.target.value) })}
                />
              )}
              {(spec.frequency === 'daily' ||
                spec.frequency === 'weekly' ||
                spec.frequency === 'monthly') && (
                <Select
                  label="Hour"
                  options={HOURS}
                  value={String(spec.hour)}
                  onChange={(event) => setSpec({ ...spec, hour: Number(event.target.value) })}
                />
              )}
              {spec.frequency === 'weekly' && (
                <Select
                  label="Day"
                  options={WEEKDAYS}
                  value={String(spec.weekday)}
                  onChange={(event) => setSpec({ ...spec, weekday: Number(event.target.value) })}
                />
              )}
              {spec.frequency === 'monthly' && (
                <Select
                  label="Day"
                  options={MONTH_DAYS}
                  value={String(spec.day)}
                  onChange={(event) => setSpec({ ...spec, day: Number(event.target.value) })}
                  hint="Stops at 28: later days are skipped in February."
                />
              )}
              <Button onClick={() => setText(preview)} icon={<Wand2 size={14} aria-hidden />}>
                Use this expression
              </Button>
            </div>
            <p className="font-mono text-xs text-fg-muted">{preview}</p>
          </div>
        </details>
      </div>
    </ToolShell>
  )
}

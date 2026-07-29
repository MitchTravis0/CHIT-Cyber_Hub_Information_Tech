/**
 * Reading a five field crontab expression: what it means, and when it next runs.
 *
 * Everything here is pure. The only thing that touches a clock is nextRuns, and
 * it takes the starting instant as an argument so a test can pin it.
 */

export type FieldName = 'minute' | 'hour' | 'dayOfMonth' | 'month' | 'dayOfWeek'

export interface CronField {
  name: FieldName
  /** The text as typed for this field, after macro expansion. */
  source: string
  /** Every value this field matches, ascending, no duplicates. */
  values: number[]
  /** The field was '*', so it places no restriction. */
  wildcard: boolean
  /** This field alone in English, e.g. 'every 5 minutes', 'Monday to Friday'. */
  english: string
}

export interface CronExpression {
  minute: CronField
  hour: CronField
  dayOfMonth: CronField
  month: CronField
  dayOfWeek: CronField
  /** The five field form, macros expanded, single spaces. */
  normalized: string
  /** The macro that was typed ('@daily'), or '' when none was. */
  macro: string
  /** The whole thing in English, one sentence ending in a full stop. */
  english: string
  /** Plain sentences about traps in this expression. */
  warnings: string[]
}

export type ParseResult =
  | { ok: true; expression: CronExpression }
  | { ok: false; error: string }

export interface DateParts {
  /** 1 to 12. */
  month: number
  /** 1 to 31. */
  day: number
  /** 0 (Sunday) to 6 (Saturday). */
  weekday: number
}

export type BuildFrequency = 'minutes' | 'hourly' | 'daily' | 'weekly' | 'monthly'

export interface BuildSpec {
  frequency: BuildFrequency
  everyMinutes: number
  minute: number
  hour: number
  weekday: number
  day: number
}

/** How many run times the page asks for. */
export const RUN_COUNT = 5
/** nextRuns gives up after this many years and returns what it has. */
export const MAX_SEARCH_YEARS = 5
/** More values than this in one field are summarised instead of listed. */
export const MAX_LISTED_VALUES = 12
/** More hour-and-minute combinations than this are described, not listed. */
export const MAX_LISTED_TIMES = 12

export const MACROS: Record<string, string> = {
  '@yearly': '0 0 1 1 *',
  '@annually': '0 0 1 1 *',
  '@monthly': '0 0 1 * *',
  '@weekly': '0 0 * * 0',
  '@daily': '0 0 * * *',
  '@midnight': '0 0 * * *',
  '@hourly': '0 * * * *',
}

export const MONTH_NAMES = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
]

export const DAY_NAMES = [
  'Sunday',
  'Monday',
  'Tuesday',
  'Wednesday',
  'Thursday',
  'Friday',
  'Saturday',
]

const MONTH_ABBR = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC']
const DAY_ABBR = ['SUN', 'MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT']

interface FieldSpec {
  name: FieldName
  /** What the field is called in a message: "is not a minute". */
  article: string
  /** What the field is called as a field: "The minute field runs from...". */
  label: string
  min: number
  /** The highest value the parser accepts, which is 7 for the day of week. */
  max: number
  /** The range as a message fragment. */
  rangeText: string
  names: string[]
  /** How one value is written in English. */
  format: (value: number) => string
  /** The unit in "every 5 minutes". */
  unit: string
  /** What "*" reads as. */
  everything: string
}

function ordinal(value: number): string {
  const tens = value % 100
  if (tens >= 11 && tens <= 13) return `${value}th`
  switch (value % 10) {
    case 1:
      return `${value}st`
    case 2:
      return `${value}nd`
    case 3:
      return `${value}rd`
    default:
      return `${value}th`
  }
}

const SPECS: Record<FieldName, FieldSpec> = {
  minute: {
    name: 'minute',
    article: 'a minute',
    label: 'minute',
    min: 0,
    max: 59,
    rangeText: '0 to 59',
    names: [],
    format: (v) => String(v),
    unit: 'minutes',
    everything: 'every minute',
  },
  hour: {
    name: 'hour',
    article: 'an hour',
    label: 'hour',
    min: 0,
    max: 23,
    rangeText: '0 to 23',
    names: [],
    format: (v) => String(v).padStart(2, '0'),
    unit: 'hours',
    everything: 'every hour',
  },
  dayOfMonth: {
    name: 'dayOfMonth',
    article: 'a day of the month',
    label: 'day-of-month',
    min: 1,
    max: 31,
    rangeText: '1 to 31',
    names: [],
    format: ordinal,
    unit: 'days',
    everything: 'every day of the month',
  },
  month: {
    name: 'month',
    article: 'a month',
    label: 'month',
    min: 1,
    max: 12,
    rangeText: '1 to 12, or JAN to DEC',
    names: MONTH_ABBR,
    format: (v) => MONTH_NAMES[v - 1],
    unit: 'months',
    everything: 'every month',
  },
  dayOfWeek: {
    name: 'dayOfWeek',
    article: 'a day of the week',
    label: 'day-of-week',
    min: 0,
    // 7 is accepted and means Sunday, so the parser's ceiling is 7 even though
    // the values it produces never are.
    max: 7,
    rangeText: '0 to 6, or SUN to SAT',
    names: DAY_ABBR,
    format: (v) => DAY_NAMES[v],
    unit: 'days of the week',
    everything: 'every day of the week',
  },
}

const ORDER: FieldName[] = ['minute', 'hour', 'dayOfMonth', 'month', 'dayOfWeek']

export function listEnglish(items: string[]): string {
  if (items.length === 0) return ''
  if (items.length === 1) return items[0]
  return `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`
}

function fail(error: string): ParseResult {
  return { ok: false, error }
}

/** The step in a field that is exactly "*\/n", or 0 when it is anything else. */
function pureStep(source: string): number {
  const match = /^\*\/(\d+)$/.exec(source)
  if (match === null) return 0
  const step = Number(match[1])
  return step > 1 ? step : 0
}

function contiguous(values: number[]): boolean {
  if (values.length < 3) return false
  for (let i = 1; i < values.length; i++) {
    if (values[i] !== values[i - 1] + 1) return false
  }
  return true
}

function fieldEnglish(spec: FieldSpec, source: string, values: number[], wildcard: boolean): string {
  if (wildcard) return spec.everything
  const step = pureStep(source)
  if (step > 0) return `every ${step} ${spec.unit}`
  if (contiguous(values)) {
    return `${spec.format(values[0])} to ${spec.format(values[values.length - 1])}`
  }
  if (values.length > MAX_LISTED_VALUES) return `${values.length} different values`
  return listEnglish(values.map(spec.format))
}

function valueOf(spec: FieldSpec, token: string): number | string {
  const trimmed = token.trim()
  if (spec.names.length > 0) {
    const index = spec.names.indexOf(trimmed.toUpperCase())
    if (index >= 0) return spec.name === 'month' ? index + 1 : index
  }
  if (!/^\d+$/.test(trimmed)) {
    if (spec.names.length > 0) {
      return `"${trimmed}" is not ${spec.article}. Use ${spec.rangeText}.`
    }
    return `"${trimmed}" is not a number. The ${spec.label} field takes ${spec.rangeText}, a range like 9-17, a list like 9,13,17, or *.`
  }
  const value = Number(trimmed)
  if (value < spec.min || value > spec.max) {
    return `${value} is not ${spec.article}. The ${spec.label} field runs from ${spec.rangeText}.`
  }
  return value
}

function parseField(name: FieldName, source: string): CronField | string {
  const spec = SPECS[name]
  const text = source.trim()
  const values = new Set<number>()

  for (const item of text.split(',')) {
    if (item.trim() === '') {
      return `"${text}" has an empty part. Remove the extra comma.`
    }
    const pieces = item.split('/')
    if (pieces.length > 2) {
      return `"${item}" is not a valid part. Use something like 5, 9-17, */15 or 9-17/2.`
    }
    let step = 1
    if (pieces.length === 2) {
      if (!/^\d+$/.test(pieces[1].trim()) || Number(pieces[1].trim()) < 1) {
        return `"${item}" is not a step. A step must be 1 or more, as in */15.`
      }
      step = Number(pieces[1].trim())
    }

    const rangeText = pieces[0].trim()
    let lo: number
    let hi: number
    if (rangeText === '*') {
      lo = spec.min
      hi = spec.max
    } else if (rangeText.includes('-')) {
      const at = rangeText.indexOf('-')
      const start = valueOf(spec, rangeText.slice(0, at))
      if (typeof start === 'string') return start
      const end = valueOf(spec, rangeText.slice(at + 1))
      if (typeof end === 'string') return end
      if (end < start) {
        return `In "${rangeText}" the end is before the start. Cron does not wrap around, so write it as two parts: ${start}-${spec.name === 'dayOfWeek' ? 6 : spec.max},${spec.min}-${end}.`
      }
      lo = start
      hi = end
    } else {
      const single = valueOf(spec, rangeText)
      if (typeof single === 'string') return single
      lo = single
      hi = pieces.length === 2 ? spec.max : single
    }

    for (let value = lo; value <= hi; value += step) {
      // 7 means Sunday, and normalising here keeps 5-7 from being read as a
      // backwards range.
      values.add(name === 'dayOfWeek' && value === 7 ? 0 : value)
    }
  }

  const sorted = [...values].sort((a, b) => a - b)
  const wildcard = text === '*'
  return {
    name,
    source: text,
    values: sorted,
    wildcard,
    english: fieldEnglish(spec, text, sorted, wildcard),
  }
}

function hoursPhrase(hour: CronField): string {
  if (hour.wildcard) return 'every hour'
  return hour.values.length === 1 ? `the hour ${hour.english}` : `the hours ${hour.english}`
}

function timeClause(minute: CronField, hour: CronField): string {
  if (minute.wildcard && hour.wildcard) return 'Every minute'
  const step = pureStep(minute.source)
  if (step > 0 && hour.wildcard) return `Every ${step} minutes`
  if (minute.wildcard) return `Every minute of ${hoursPhrase(hour)}`
  if (!hour.wildcard && minute.values.length * hour.values.length <= MAX_LISTED_TIMES) {
    const times: string[] = []
    for (const h of hour.values) {
      for (const m of minute.values) {
        times.push(`${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`)
      }
    }
    return `At ${listEnglish(times)}`
  }
  return `At ${minute.english} past ${hoursPhrase(hour)}`
}

function sentenceFor(fields: Record<FieldName, CronField>): string {
  const parts = [timeClause(fields.minute, fields.hour)]
  if (!fields.dayOfMonth.wildcard) {
    parts.push(`on the ${fields.dayOfMonth.english} of the month`)
  }
  if (!fields.dayOfWeek.wildcard) {
    parts.push(
      fields.dayOfMonth.wildcard
        ? `on ${fields.dayOfWeek.english}`
        : `and also on ${fields.dayOfWeek.english}`,
    )
  }
  if (!fields.month.wildcard) parts.push(`in ${fields.month.english}`)
  return `${parts.join(', ')}.`
}

function warningsFor(fields: Record<FieldName, CronField>): string[] {
  const out: string[] = []
  if (!fields.dayOfMonth.wildcard && !fields.dayOfWeek.wildcard) {
    out.push(
      `Both the day-of-month and the day-of-week fields are set. Cron treats that as "or", not "and": this runs on ${fields.dayOfMonth.english} of the month AND on ${fields.dayOfWeek.english}, not only when the two fall on the same day.`,
    )
  }

  const minuteStep = pureStep(fields.minute.source)
  if (minuteStep > 0 && 60 % minuteStep !== 0) {
    const values = fields.minute.values
    const last = values[values.length - 1]
    out.push(
      `"*/${minuteStep}" in the minutes field is not the same as "every ${minuteStep} minutes". It runs at ${listEnglish(values.map(String))} past the hour, then again at 0, which is only ${60 - last} minutes later.`,
    )
  }

  const hourStep = pureStep(fields.hour.source)
  if (hourStep > 0 && 24 % hourStep !== 0) {
    const values = fields.hour.values
    const last = values[values.length - 1]
    out.push(
      `"*/${hourStep}" in the hours field restarts at midnight, so the gap between the last run of one day and the first of the next is ${24 - last} hours, not ${hourStep}.`,
    )
  }

  const dayStep = pureStep(fields.dayOfMonth.source)
  if (dayStep > 0) {
    out.push(
      `"*/${dayStep}" in the day-of-month field restarts on the 1st of every month, so the gap across the end of a month is not ${dayStep} days.`,
    )
  }

  return out
}

export function parseCron(input: string): ParseResult {
  const text = input.trim()
  if (text === '') {
    return fail('Type a cron expression above, for example 30 2 * * 1-5.')
  }

  let macro = ''
  let body = text
  if (text.startsWith('@')) {
    const lower = text.toLowerCase()
    if (lower === '@reboot') {
      return fail(
        '@reboot runs the job when the machine starts up, so there is no schedule to show and no next run time.',
      )
    }
    const expanded = MACROS[lower]
    if (expanded === undefined) {
      return fail(
        `"${text}" is not a cron macro. The ones cron understands are @yearly, @monthly, @weekly, @daily and @hourly.`,
      )
    }
    macro = lower
    body = expanded
  }

  const parts = body.split(/\s+/)
  if (parts.length !== 5) {
    if (parts.length === 6) {
      return fail(
        'That looks like a six field expression, which is the Quartz or Spring format with seconds first. CHIT reads the five field crontab format. Drop the first field and try again.',
      )
    }
    if (parts.length === 7) {
      return fail(
        'That looks like a seven field expression, which is the Quartz format with seconds first and a year last. CHIT reads the five field crontab format. Keep the middle five fields and try again.',
      )
    }
    return fail(
      `A cron expression has five fields separated by spaces: minute, hour, day of month, month, day of week. That one has ${parts.length}.`,
    )
  }

  const fields = {} as Record<FieldName, CronField>
  for (let i = 0; i < ORDER.length; i++) {
    const parsed = parseField(ORDER[i], parts[i])
    if (typeof parsed === 'string') return fail(parsed)
    fields[ORDER[i]] = parsed
  }

  return {
    ok: true,
    expression: {
      ...fields,
      normalized: parts.join(' '),
      macro,
      english: sentenceFor(fields),
      warnings: warningsFor(fields),
    },
  }
}

export function matchesDay(expression: CronExpression, parts: DateParts): boolean {
  if (!expression.month.values.includes(parts.month)) return false
  const { dayOfMonth, dayOfWeek } = expression
  if (dayOfMonth.wildcard && dayOfWeek.wildcard) return true
  if (dayOfMonth.wildcard) return dayOfWeek.values.includes(parts.weekday)
  if (dayOfWeek.wildcard) return dayOfMonth.values.includes(parts.day)
  // Vixie cron reads two restricted day fields as "or", which is the single
  // most misread thing about crontab.
  return dayOfMonth.values.includes(parts.day) || dayOfWeek.values.includes(parts.weekday)
}

export function nextRuns(expression: CronExpression, from: Date, count: number): Date[] {
  const out: Date[] = []
  const start = new Date(from.getTime())
  start.setSeconds(0, 0)
  start.setMinutes(start.getMinutes() + 1)

  const limit = new Date(from.getTime())
  limit.setFullYear(limit.getFullYear() + MAX_SEARCH_YEARS)

  const day = new Date(start.getFullYear(), start.getMonth(), start.getDate())
  while (out.length < count && day.getTime() <= limit.getTime()) {
    const matches = matchesDay(expression, {
      month: day.getMonth() + 1,
      day: day.getDate(),
      weekday: day.getDay(),
    })
    if (matches) {
      for (const hour of expression.hour.values) {
        for (const minute of expression.minute.values) {
          const candidate = new Date(
            day.getFullYear(),
            day.getMonth(),
            day.getDate(),
            hour,
            minute,
            0,
            0,
          )
          // A local time that does not exist (the hour the clocks go forward)
          // lands on the next real instant, which may be the following day.
          if (candidate.getDate() !== day.getDate()) continue
          if (candidate.getTime() < start.getTime()) continue
          if (candidate.getTime() > limit.getTime()) break
          out.push(candidate)
          if (out.length >= count) break
        }
        if (out.length >= count) break
      }
    }
    day.setDate(day.getDate() + 1)
  }
  return out
}

function plural(count: number, word: string): string {
  return `${count} ${word}${count === 1 ? '' : 's'}`
}

export function untilText(from: Date, to: Date): string {
  const ms = to.getTime() - from.getTime()
  if (ms <= 0) return 'now'
  const totalMinutes = Math.floor(ms / 60000)
  if (totalMinutes < 1) return 'in less than a minute'
  const days = Math.floor(totalMinutes / 1440)
  const hours = Math.floor((totalMinutes % 1440) / 60)
  const minutes = totalMinutes % 60
  if (days > 0) {
    return hours > 0 ? `in ${plural(days, 'day')} ${plural(hours, 'hour')}` : `in ${plural(days, 'day')}`
  }
  if (hours > 0) {
    return minutes > 0
      ? `in ${plural(hours, 'hour')} ${plural(minutes, 'minute')}`
      : `in ${plural(hours, 'hour')}`
  }
  return `in ${plural(minutes, 'minute')}`
}

export function buildExpression(spec: BuildSpec): string {
  switch (spec.frequency) {
    case 'minutes':
      return `*/${spec.everyMinutes} * * * *`
    case 'hourly':
      return `${spec.minute} * * * *`
    case 'daily':
      return `${spec.minute} ${spec.hour} * * *`
    case 'weekly':
      return `${spec.minute} ${spec.hour} * * ${spec.weekday}`
    case 'monthly':
      return `${spec.minute} ${spec.hour} ${spec.day} * *`
  }
}

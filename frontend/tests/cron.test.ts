// Every Date below is built in local time, so the zone is pinned before
// anything reads a clock. Node re-reads TZ when it is assigned.
process.env.TZ = 'UTC'

import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildExpression,
  listEnglish,
  MACROS,
  matchesDay,
  MAX_LISTED_TIMES,
  MAX_LISTED_VALUES,
  MAX_SEARCH_YEARS,
  nextRuns,
  parseCron,
  RUN_COUNT,
  untilText,
  type CronExpression,
} from '../src/tools/cron-explainer/cron.ts'

function ok(input: string): CronExpression {
  const result = parseCron(input)
  assert.equal(result.ok, true, `expected ${input} to parse: ${result.ok ? '' : result.error}`)
  if (!result.ok) throw new Error('unreachable')
  return result.expression
}

function err(input: string): string {
  const result = parseCron(input)
  assert.equal(result.ok, false, `expected ${input} to be rejected`)
  if (result.ok) throw new Error('unreachable')
  return result.error
}

function iso(dates: Date[]): string[] {
  return dates.map((date) => date.toISOString().replace('.000Z', 'Z'))
}

test('parses every field shape', () => {
  assert.deepEqual(ok('* * * * *').minute.values.length, 60)
  assert.deepEqual(ok('5 * * * *').minute.values, [5])
  assert.deepEqual(ok('1-3 * * * *').minute.values, [1, 2, 3])
  assert.deepEqual(ok('*/15 * * * *').minute.values, [0, 15, 30, 45])
  assert.deepEqual(ok('10-20/5 * * * *').minute.values, [10, 15, 20])
  assert.deepEqual(ok('5/20 * * * *').minute.values, [5, 25, 45])
  assert.deepEqual(ok('1,3,5 * * * *').minute.values, [1, 3, 5])
  assert.deepEqual(ok('1-3,7,9-11 * * * *').minute.values, [1, 2, 3, 7, 9, 10, 11])
})

test('sorts and deduplicates values', () => {
  assert.deepEqual(ok('5,1,5,3 * * * *').minute.values, [1, 3, 5])
  assert.deepEqual(ok('1-3,2-4 * * * *').minute.values, [1, 2, 3, 4])
})

test('accepts month and day names in any case', () => {
  assert.deepEqual(ok('0 0 * jan-mar *').month.values, [1, 2, 3])
  assert.deepEqual(ok('0 0 * DEC *').month.values, [12])
  assert.deepEqual(ok('0 0 * * MON,wed,Fri').dayOfWeek.values, [1, 3, 5])
})

test('treats day-of-week 7 as Sunday', () => {
  assert.deepEqual(ok('0 0 * * 7').dayOfWeek.values, [0])
  assert.deepEqual(ok('0 0 * * 5-7').dayOfWeek.values, [0, 5, 6])
  assert.deepEqual(ok('0 0 * * 0,7').dayOfWeek.values, [0])
})

test('expands every macro', () => {
  assert.deepEqual(MACROS, {
    '@yearly': '0 0 1 1 *',
    '@annually': '0 0 1 1 *',
    '@monthly': '0 0 1 * *',
    '@weekly': '0 0 * * 0',
    '@daily': '0 0 * * *',
    '@midnight': '0 0 * * *',
    '@hourly': '0 * * * *',
  })
  assert.equal(ok('@daily').normalized, '0 0 * * *')
  assert.equal(ok('@daily').macro, '@daily')
  assert.equal(ok('@DAILY').macro, '@daily')
  assert.equal(ok('@yearly').normalized, '0 0 1 1 *')
  assert.equal(ok('@weekly').normalized, '0 0 * * 0')
  assert.equal(ok('@hourly').normalized, '0 * * * *')
  assert.equal(ok('30 2 * * *').macro, '')
})

test('wildcard is only a literal star', () => {
  assert.equal(ok('* * * * *').minute.wildcard, true)
  assert.equal(ok('0-59 * * * *').minute.wildcard, false)
  assert.equal(ok('0-59 * * * *').minute.values.length, 60)
  assert.equal(ok('*/1 * * * *').minute.wildcard, false)
})

test('rejects everything in section 8 with its exact message', () => {
  assert.equal(err(''), 'Type a cron expression above, for example 30 2 * * 1-5.')
  assert.equal(err('   '), 'Type a cron expression above, for example 30 2 * * 1-5.')
  assert.equal(
    err('* * * *'),
    'A cron expression has five fields separated by spaces: minute, hour, day of month, month, day of week. That one has 4.',
  )
  assert.equal(
    err('0 30 2 * * *'),
    'That looks like a six field expression, which is the Quartz or Spring format with seconds first. CHIT reads the five field crontab format. Drop the first field and try again.',
  )
  assert.equal(
    err('0 30 2 * * * 2026'),
    'That looks like a seven field expression, which is the Quartz format with seconds first and a year last. CHIT reads the five field crontab format. Keep the middle five fields and try again.',
  )
  assert.equal(
    err('@reboot'),
    '@reboot runs the job when the machine starts up, so there is no schedule to show and no next run time.',
  )
  assert.equal(
    err('@weekley'),
    '"@weekley" is not a cron macro. The ones cron understands are @yearly, @monthly, @weekly, @daily and @hourly.',
  )
  assert.equal(err('61 * * * *'), '61 is not a minute. The minute field runs from 0 to 59.')
  assert.equal(err('0 24 * * *'), '24 is not an hour. The hour field runs from 0 to 23.')
  assert.equal(
    err('0 0 * MON *'),
    '"MON" is not a month. Use 1 to 12, or JAN to DEC.',
  )
  assert.equal(
    err('0 0 * * JAN'),
    '"JAN" is not a day of the week. Use 0 to 6, or SUN to SAT.',
  )
  assert.equal(
    err('0 x * * *'),
    '"x" is not a number. The hour field takes 0 to 23, a range like 9-17, a list like 9,13,17, or *.',
  )
  assert.equal(
    err('50-10 * * * *'),
    'In "50-10" the end is before the start. Cron does not wrap around, so write it as two parts: 50-59,0-10.',
  )
  assert.equal(
    err('*/0 * * * *'),
    '"*/0" is not a step. A step must be 1 or more, as in */15.',
  )
  assert.equal(err('1,,2 * * * *'), '"1,,2" has an empty part. Remove the extra comma.')
  assert.equal(
    err('1/2/3 * * * *'),
    '"1/2/3" is not a valid part. Use something like 5, 9-17, */15 or 9-17/2.',
  )
})

test('field english reads correctly', () => {
  assert.equal(ok('* * * * *').minute.english, 'every minute')
  assert.equal(ok('* * * * *').dayOfWeek.english, 'every day of the week')
  assert.equal(ok('*/5 * * * *').minute.english, 'every 5 minutes')
  assert.equal(ok('0 9-17 * * *').hour.english, '09 to 17')
  assert.equal(ok('0 0 * * 1-5').dayOfWeek.english, 'Monday to Friday')
  // Three is the smallest run that reads as a range, two is still a list.
  assert.equal(ok('1-3 * * * *').minute.english, '1 to 3')
  assert.equal(ok('1,2,3 * * * *').minute.english, '1 to 3')
  assert.equal(ok('1-2 * * * *').minute.english, '1 and 2')
  assert.equal(ok('1,5 * * * *').minute.english, '1 and 5')
  assert.equal(ok('0 0 1,15 * *').dayOfMonth.english, '1st and 15th')
  assert.equal(ok('0 0 * 1,7 *').month.english, 'January and July')
  // 13 values is one past MAX_LISTED_VALUES, and 1-13 is contiguous so the
  // summary only kicks in for a list that is not a run.
  assert.equal(MAX_LISTED_VALUES, 12)
  assert.equal(
    ok('1,3,5,7,9,11,13,15,17,19,21,23,25 * * * *').minute.english,
    '13 different values',
  )
  assert.equal(ok('1,3,5,7,9,11,13,15,17,19,21,23 * * * *').minute.english.startsWith('1, 3, 5'), true)
  assert.equal(
    ok('0 0 1,2,3,4,11,12,13,21,22,23,31 * *').dayOfMonth.english,
    '1st, 2nd, 3rd, 4th, 11th, 12th, 13th, 21st, 22nd, 23rd and 31st',
  )
})

test('listEnglish joins without an Oxford comma', () => {
  assert.equal(listEnglish([]), '')
  assert.equal(listEnglish(['a']), 'a')
  assert.equal(listEnglish(['a', 'b']), 'a and b')
  assert.equal(listEnglish(['a', 'b', 'c']), 'a, b and c')
})

test('the sentence matches the table', () => {
  assert.equal(MAX_LISTED_TIMES, 12)
  assert.equal(ok('* * * * *').english, 'Every minute.')
  assert.equal(ok('*/5 * * * *').english, 'Every 5 minutes.')
  assert.equal(ok('30 2 * * *').english, 'At 02:30.')
  assert.equal(
    ok('0 9-17 * * 1-5').english,
    'At 09:00, 10:00, 11:00, 12:00, 13:00, 14:00, 15:00, 16:00 and 17:00, on Monday to Friday.',
  )
  assert.equal(
    ok('0 3 1,15 * 2').english,
    'At 03:00, on the 1st and 15th of the month, and also on Tuesday.',
  )
  assert.equal(ok('0 0 1 1 *').english, 'At 00:00, on the 1st of the month, in January.')
  assert.equal(ok('15,45 * * * *').english, 'At 15 and 45 past every hour.')
  assert.equal(ok('* 9 * * *').english, 'Every minute of the hour 09.')
  assert.equal(ok('0 0 * * 0').english, 'At 00:00, on Sunday.')
})

test('warns about the or rule', () => {
  const warnings = ok('0 3 1 * 1').warnings
  assert.equal(warnings.length, 1)
  assert.ok(warnings[0].includes('"or", not "and"'), warnings[0])
  assert.equal(
    warnings[0],
    'Both the day-of-month and the day-of-week fields are set. Cron treats that as "or", not "and": this runs on 1st of the month AND on Monday, not only when the two fall on the same day.',
  )
  assert.deepEqual(ok('0 3 1 * *').warnings, [])
  assert.deepEqual(ok('0 3 * * 1').warnings, [])
})

test('warns about a step that does not divide', () => {
  const minute = ok('*/7 * * * *').warnings
  assert.equal(minute.length, 1)
  assert.equal(
    minute[0],
    '"*/7" in the minutes field is not the same as "every 7 minutes". It runs at 0, 7, 14, 21, 28, 35, 42, 49 and 56 past the hour, then again at 0, which is only 4 minutes later.',
  )
  assert.deepEqual(ok('*/15 * * * *').warnings, [])

  const hour = ok('0 */5 * * *').warnings
  assert.equal(hour.length, 1)
  assert.equal(
    hour[0],
    '"*/5" in the hours field restarts at midnight, so the gap between the last run of one day and the first of the next is 4 hours, not 5.',
  )
  assert.deepEqual(ok('0 */6 * * *').warnings, [])

  const day = ok('0 0 */5 * *').warnings
  assert.equal(day.length, 1)
  assert.equal(
    day[0],
    '"*/5" in the day-of-month field restarts on the 1st of every month, so the gap across the end of a month is not 5 days.',
  )
})

test('matchesDay applies the or rule', () => {
  const both = ok('0 3 1 * 1')
  assert.equal(matchesDay(both, { month: 6, day: 1, weekday: 3 }), true)
  assert.equal(matchesDay(both, { month: 6, day: 8, weekday: 1 }), true)
  assert.equal(matchesDay(both, { month: 6, day: 8, weekday: 3 }), false)

  const domOnly = ok('0 3 1 * *')
  assert.equal(matchesDay(domOnly, { month: 6, day: 1, weekday: 3 }), true)
  assert.equal(matchesDay(domOnly, { month: 6, day: 8, weekday: 1 }), false)

  const dowOnly = ok('0 3 * * 1')
  assert.equal(matchesDay(dowOnly, { month: 6, day: 1, weekday: 3 }), false)
  assert.equal(matchesDay(dowOnly, { month: 6, day: 8, weekday: 1 }), true)

  const everything = ok('* * * * *')
  for (let month = 1; month <= 12; month++) {
    for (let day = 1; day <= 31; day++) {
      for (let weekday = 0; weekday <= 6; weekday++) {
        assert.equal(matchesDay(everything, { month, day, weekday }), true)
      }
    }
  }
})

test('matchesDay honours the month field', () => {
  const june = ok('0 0 1 6 *')
  assert.equal(matchesDay(june, { month: 6, day: 1, weekday: 0 }), true)
  assert.equal(matchesDay(june, { month: 7, day: 1, weekday: 0 }), false)
})

// Every expected list below came out of a brute-force python script that steps
// one minute at a time and tests the fields directly, not out of this code.
test('nextRuns finds the next five', () => {
  assert.equal(RUN_COUNT, 5)
  const runs = nextRuns(ok('30 2 * * *'), new Date('2026-03-10T09:17:00Z'), 5)
  assert.deepEqual(iso(runs), [
    '2026-03-11T02:30:00Z',
    '2026-03-12T02:30:00Z',
    '2026-03-13T02:30:00Z',
    '2026-03-14T02:30:00Z',
    '2026-03-15T02:30:00Z',
  ])
})

test('nextRuns starts strictly after from', () => {
  const runs = nextRuns(ok('30 2 * * *'), new Date('2026-03-10T02:30:00Z'), 1)
  assert.deepEqual(iso(runs), ['2026-03-11T02:30:00Z'])
})

test('nextRuns zeroes seconds', () => {
  const runs = nextRuns(ok('30 2 * * *'), new Date('2026-03-10T02:29:59.500Z'), 1)
  assert.deepEqual(iso(runs), ['2026-03-10T02:30:00Z'])
})

test('nextRuns handles a weekday schedule', () => {
  const runs = nextRuns(ok('0 9 * * 1'), new Date('2026-03-11T12:00:00Z'), 5)
  assert.deepEqual(iso(runs), [
    '2026-03-16T09:00:00Z',
    '2026-03-23T09:00:00Z',
    '2026-03-30T09:00:00Z',
    '2026-04-06T09:00:00Z',
    '2026-04-13T09:00:00Z',
  ])
})

test('nextRuns handles the or rule', () => {
  const runs = nextRuns(ok('0 0 1 * 1'), new Date('2026-06-15T00:00:00Z'), 6)
  assert.deepEqual(iso(runs), [
    '2026-06-22T00:00:00Z',
    '2026-06-29T00:00:00Z',
    '2026-07-01T00:00:00Z',
    '2026-07-06T00:00:00Z',
    '2026-07-13T00:00:00Z',
    '2026-07-20T00:00:00Z',
  ])
})

test('nextRuns returns nothing for an impossible date', () => {
  assert.deepEqual(nextRuns(ok('0 0 30 2 *'), new Date('2026-01-01T00:00:00Z'), 5), [])
  assert.deepEqual(nextRuns(ok('0 0 31 4 *'), new Date('2026-01-01T00:00:00Z'), 5), [])
})

test('nextRuns finds a leap day', () => {
  assert.equal(MAX_SEARCH_YEARS, 5)
  const runs = nextRuns(ok('0 0 29 2 *'), new Date('2026-03-01T00:00:00Z'), 1)
  assert.deepEqual(iso(runs), ['2028-02-29T00:00:00Z'])
})

test('untilText reads in plain words', () => {
  const base = new Date('2026-03-10T00:00:00Z')
  const at = (ms: number) => untilText(base, new Date(base.getTime() + ms))
  assert.equal(at(45_000), 'in less than a minute')
  assert.equal(at(60_000), 'in 1 minute')
  assert.equal(at(12 * 60_000), 'in 12 minutes')
  assert.equal(at(65 * 60_000), 'in 1 hour 5 minutes')
  assert.equal(at(3 * 3_600_000), 'in 3 hours')
  assert.equal(at(52 * 3_600_000), 'in 2 days 4 hours')
  assert.equal(at(48 * 3_600_000), 'in 2 days')
  assert.equal(at(-60_000), 'now')
  assert.equal(at(0), 'now')
})

test('buildExpression covers every frequency', () => {
  const base = { frequency: 'daily' as const, everyMinutes: 15, minute: 0, hour: 0, weekday: 0, day: 1 }
  assert.equal(buildExpression({ ...base, frequency: 'minutes', everyMinutes: 15 }), '*/15 * * * *')
  assert.equal(buildExpression({ ...base, frequency: 'hourly', minute: 5 }), '5 * * * *')
  assert.equal(buildExpression({ ...base, frequency: 'daily', minute: 30, hour: 2 }), '30 2 * * *')
  assert.equal(
    buildExpression({ ...base, frequency: 'weekly', minute: 0, hour: 7, weekday: 1 }),
    '0 7 * * 1',
  )
  assert.equal(
    buildExpression({ ...base, frequency: 'monthly', minute: 15, hour: 4, day: 1 }),
    '15 4 1 * *',
  )

  for (const frequency of ['minutes', 'hourly', 'daily', 'weekly', 'monthly'] as const) {
    const built = buildExpression({ ...base, frequency })
    assert.equal(parseCron(built).ok, true, `${frequency} built ${built}`)
  }
})

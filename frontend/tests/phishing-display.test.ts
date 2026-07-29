import test from 'node:test'
import assert from 'node:assert/strict'
import { hostSlug, levelTone, severityTone } from '../src/tools/phishing-inspector/display.ts'

test('hostSlug turns a host into a safe file name', () => {
  assert.equal(hostSlug('www.example.co.uk'), 'www-example-co-uk')
  assert.equal(hostSlug('192.168.1.1'), '192-168-1-1')
  // A run of characters outside [a-z0-9] collapses to one hyphen, and "-" is
  // itself outside that set, so the xn-- prefix comes out as a single hyphen.
  assert.equal(hostSlug('xn--80ak6aa92e.com'), 'xn-80ak6aa92e-com')
  assert.equal(hostSlug('EXAMPLE.COM'), 'example-com')
  assert.equal(hostSlug(''), 'link')
  assert.equal(hostSlug('...'), 'link')
})

test('hostSlug truncates a long host without leaving a trailing hyphen', () => {
  const host = 'a'.repeat(39) + '.' + 'b'.repeat(20)
  const slug = hostSlug(host)
  assert.equal(slug.length, 39)
  assert.equal(slug, 'a'.repeat(39))
  assert.ok(!slug.endsWith('-'))
})

test('severityTone maps every severity, including one it has never seen', () => {
  assert.equal(severityTone('danger'), 'danger')
  assert.equal(severityTone('warn'), 'warn')
  assert.equal(severityTone('info'), 'info')
  assert.equal(severityTone('catastrophe'), 'info')
  assert.equal(severityTone(''), 'info')
})

// A chain nothing answered is level "unknown". It must not render as the red
// danger banner: nothing was found wrong, and nothing was checked either.
test('levelTone keeps unknown out of the danger banner', () => {
  assert.equal(levelTone('ok'), 'ok')
  assert.equal(levelTone('warn'), 'warn')
  assert.equal(levelTone('danger'), 'danger')
  assert.equal(levelTone('unknown'), 'unknown')
})

test('levelTone treats anything it does not know as unknown, never as danger', () => {
  for (const level of ['', 'info', 'OK', 'Danger', 'something-new', 'error']) {
    assert.equal(levelTone(level), 'unknown', `${level} should be unknown`)
  }
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { LISTENER_TOOLS, TOUR_STEPS } from '../src/shell/tour.ts'
import { TOOLS } from '../src/tools/registry.ts'

test('there are five steps and their ids are unique', () => {
  assert.equal(TOUR_STEPS.length, 5)
  const ids = TOUR_STEPS.map((step) => step.id)
  assert.equal(new Set(ids).size, ids.length)
})

test('every step has a title and a body worth reading', () => {
  for (const step of TOUR_STEPS) {
    assert.ok(step.title.trim().length > 0, `${step.id} has no title`)
    assert.ok(step.body.trim().length > 40, `${step.id} has a one-liner for a body`)
    // The house style, and the user's standing rule.
    assert.ok(!step.title.includes('—'), `${step.id} title has an em dash`)
    assert.ok(!step.body.includes('—'), `${step.id} body has an em dash`)
  }
})

// The reason a tour was asked for at all. Phase 5 raised it for LAN File Drop and
// Phase 9 for the two new listeners: a junior tech has to be told, once, that
// these three open a port and that Stop closes it.
test('a step names every tool that opens an inbound port', () => {
  const step = TOUR_STEPS.find((each) => each.tools !== undefined)
  assert.ok(step, 'no step declares which tools it is about')
  assert.deepEqual(step!.tools, LISTENER_TOOLS)
  assert.equal(LISTENER_TOOLS.length, 3)

  for (const id of LISTENER_TOOLS) {
    const tool = TOOLS.find((each) => each.id === id)
    assert.ok(tool, `${id} is in the tour but not in the registry`)
    assert.ok(
      step!.body.includes(tool!.name),
      `the listener step does not name ${tool!.name} in its text`,
    )
  }
  assert.ok(step!.body.includes('Stop'), 'the listener step never mentions the Stop button')
})

// A renamed or removed tool must not leave the tour pointing at nothing.
test('every tool id any step mentions exists in the registry', () => {
  const known = new Set(TOOLS.map((tool) => tool.id))
  for (const step of TOUR_STEPS) {
    for (const id of step.tools ?? []) {
      assert.ok(known.has(id), `${step.id} names ${id}, which is not a tool`)
    }
  }
})

// LISTENER_TOOLS is the list the tour is built from, so it has to be the real
// set. These three are the only tools in the app that bind a port; if a fourth
// is ever added, this fails and the tour gets updated with it.
test('the listener list is exactly the tools that bind a port', () => {
  assert.deepEqual([...LISTENER_TOOLS].sort(), [
    'lan-file-drop',
    'lan-throughput',
    'port-listener',
  ])
})

test('the first step says no install and no admin rights', () => {
  const body = TOUR_STEPS[0].body.toLowerCase()
  assert.ok(body.includes('administrator') || body.includes('admin'))
  assert.ok(body.includes('install'))
})

test('a step explains the keyboard shortcut for searching', () => {
  const step = TOUR_STEPS.find((each) => each.body.includes('+K'))
  assert.ok(step, 'no step tells the tech about the command palette shortcut')
})

test('a step explains where the data is kept and what portable mode is', () => {
  const step = TOUR_STEPS.find((each) => each.body.includes('portable.txt'))
  assert.ok(step, 'no step explains portable mode')
})

// Bundled with esbuild into /tmp and executed by node --test; no browser or production backend.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { dateWindow, shiftDate, searchBlocks, DayStore, recoverDay } from './dual-notes-core'

const day = (text = 'initial', revision = 0) => ({ date: '2026-10-05', revision, rows: [{ id: 'first', left: text, right: '' }] })

test('calendar pages count backward from today without gaps, including leap days and DST boundaries', () => {
  assert.deepEqual(dateWindow('2026-10-05', 'focus', 8), ['2026-10-05'])
  const current = dateWindow('2026-10-05', 'overview')
  const previous = dateWindow('2026-10-05', 'overview', 1)
  assert.equal(current.length, 10)
  assert.equal(current[0], '2026-09-26')
  assert.equal(current[9], '2026-10-05')
  assert.equal(previous[9], shiftDate(current[0], -1))
  assert.equal(shiftDate('2024-03-01', -1), '2024-02-29')
  assert.equal(shiftDate('2026-03-08', 1), '2026-03-09')
})

test('literal search crosses rich marks, includes code lines, preserves positions and is case insensitive', () => {
  const blocks = [
    { text: 'hello MARK world', positions: Array.from({ length: 16 }, (_, i) => i + 1) },
    { text: 'const marker = "[a.b]"\nMARK', positions: Array.from({ length: 25 }, (_, i) => i + 30) },
  ]
  assert.equal(searchBlocks(blocks, 'mark').length, 3)
  assert.equal(searchBlocks(blocks, '[a.b]').length, 1)
  assert.deepEqual(searchBlocks(blocks, ''), [])
  assert.equal(searchBlocks(blocks, 'hello MARK')[0].to, 11)
})

test('edits during an in-flight save serialize into another save with the returned revision', async () => {
  const calls = []
  let resolveFirst = () => {}
  const store = new DayStore(async snapshot => {
    calls.push(snapshot)
    if (calls.length === 1) return new Promise(resolve => { resolveFirst = resolve })
    return { ...snapshot, revision: snapshot.revision + 1 }
  }, () => {})
  store.register(day())
  store.edit(day().date, value => { value.rows[0].left = 'first edit' })
  const saving = store.save(day().date)
  store.edit(day().date, value => { value.rows[0].left = 'last keystroke' })
  assert.strictEqual(store.save(day().date), saving)
  resolveFirst({ ...calls[0], revision: 1 })
  await saving
  assert.equal(calls.length, 2)
  assert.equal(calls[1].revision, 1)
  assert.equal(calls[1].rows[0].left, 'last keystroke')
  assert.equal(store.states.get(day().date).day.revision, 2)
  assert.equal(store.states.get(day().date).dirty, false)
})

test('a late load cannot roll a successfully saved revision or its content backward', async () => {
  const store = new DayStore(async value => ({ ...value, revision: value.revision + 1 }), () => {})
  store.register(day('old', 1))
  store.edit(day().date, value => { value.rows[0].left = 'latest' })
  await store.save(day().date)
  const state = store.register(day('old', 1))
  assert.equal(state.day.revision, 2)
  assert.equal(state.day.rows[0].left, 'latest')
})

test('failed saves keep drafts and a manual retry can succeed', async () => {
  let fail = true
  const store = new DayStore(async value => {
    if (fail) throw new Error('offline')
    return { ...value, revision: 1 }
  }, () => {})
  store.register(day())
  store.edit(day().date, value => { value.rows[0].left = 'keep me' })
  await assert.rejects(store.flush())
  assert.equal(store.states.get(day().date).dirty, true)
  assert.equal(store.states.get(day().date).day.rows[0].left, 'keep me')
  fail = false
  await store.flush()
  assert.equal(store.states.get(day().date).dirty, false)
})

test('revision conflicts never silently overwrite saved data, including recovered drafts', async () => {
  let calls = 0
  const store = new DayStore(async () => { calls++; throw Object.assign(new Error('conflict'), { status: 409 }) }, () => {})
  store.register(day())
  store.edit(day().date, value => { value.rows[0].left = 'unsaved draft' })
  await assert.rejects(store.save(day().date))
  await assert.rejects(store.save(day().date))
  assert.equal(calls, 1)
  assert.equal(store.states.get(day().date).day.rows[0].left, 'unsaved draft')
  const restored = new DayStore(async value => value, () => {}).register(day('server', 3), day('local', 2))
  assert.equal(restored.conflict, true)
  assert.equal(restored.day.rows[0].left, 'local')
})

test('conflict reload preserves edits committed during a delayed load before replacing the target day', async () => {
  const store = new DayStore(async value => value, () => {})
  store.register(day('server', 2), day('draft', 1))
  let respond = () => {}
  const backups = []
  const recovery = recoverDay(store, day().date, draft => backups.push(draft),
    date => new Promise(resolve => { assert.equal(date, day().date); respond = resolve }), async () => {})
  store.edit(day().date, value => { value.rows[0].left = 'last input during load' })
  respond(day('new server', 3))
  await recovery
  assert.deepEqual(backups.map(draft => draft.rows[0].left), ['draft', 'last input during load'])
  assert.equal(store.states.get(day().date).day.rows[0].left, 'new server')
})

test('failed recovery backup or mismatched date never discards the live draft', async () => {
  const store = new DayStore(async value => value, () => {})
  store.register(day('server', 2), day('draft', 1))
  await assert.rejects(recoverDay(store, day().date, () => { throw new Error('quota') }, async () => day(), async () => {}))
  assert.equal(store.states.get(day().date).day.rows[0].left, 'draft')
  await assert.rejects(recoverDay(store, day().date, () => {}, async () => ({ ...day(), date: '2026-10-04' }), async () => {}))
  assert.equal(store.states.get(day().date).day.rows[0].left, 'draft')
})

test('failure while backing up the last input keeps the latest live draft and leaves replacement unapplied', async () => {
  const store = new DayStore(async value => value, () => {})
  store.register(day('server', 2), day('draft', 1))
  let backups = 0
  await assert.rejects(recoverDay(store, day().date, () => {
    backups++
    if (backups === 2) throw new Error('quota while preserving final input')
  }, async () => day('new server', 3), async () => {
    store.edit(day().date, value => { value.rows[0].left = 'last input' })
  }))
  assert.equal(backups, 2)
  assert.equal(store.states.get(day().date).day.rows[0].left, 'last input')
  assert.equal(store.states.get(day().date).day.revision, 1)
  assert.equal(store.states.get(day().date).dirty, true)
})

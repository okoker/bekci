// Pins the twelve ready-made alert rules (src/help/recipes.js) and applyConditions
// (src/help/applyConditions.js): the data is in the form's own shape and passes its
// validation, and applying one replaces the form's conditions with fresh copies. Pure, Node only.
import test from 'node:test'
import assert from 'node:assert/strict'
import { recipes } from '../src/help/recipes.js'
import { applyConditions } from '../src/help/applyConditions.js'
import { explainRule } from '../src/help/explainRule.js'
import {
  getDefaultCondition, getDefaultConfig, minFailWindow, failWindowError, valueError,
} from '../src/conditionRules.js'

const byId = (id) => recipes.find((r) => r.id === id)
const clone = (x) => JSON.parse(JSON.stringify(x))
const FORM_KEYS = Object.keys(getDefaultCondition()).sort()

// Bekci's PRIMARY check: the first check of the preferred type by name, in byte order
// (SQLite's default ORDER BY name; internal/store/checks.go PrimaryCheck).
function primaryName(r) {
  const names = r.conditions.filter((c) => c.check_type === r.preferred).map((c) => c.check_name)
  return names.sort((a, b) => Buffer.compare(Buffer.from(a), Buffer.from(b)))[0]
}

// What the person types into each field the recipe leaves empty.
const FILLED = { 'config.port': 25, 'config.expect_value': '203.0.113.10', 'config.endpoint': '/admin' }
function setPath(cond, path, value) {
  const [a, b] = path.split('.')
  cond[a][b] = value
}
function filled(r) {
  const conds = clone(r.conditions)
  for (const n of r.needs) setPath(conds[n.condition], n.field, FILLED[n.field])
  return conds
}

test('twelve recipes, B1 to S4, four on each shelf', () => {
  assert.deepEqual(recipes.map((r) => r.id),
    ['B1', 'B2', 'B3', 'B4', 'C1', 'C2', 'C3', 'C4', 'S1', 'S2', 'S3', 'S4'])
  for (const [shelf, prefix] of [['basic', 'B'], ['combined', 'C'], ['security', 'S']]) {
    const ids = recipes.filter((r) => r.shelf === shelf).map((r) => r.id)
    assert.equal(ids.length, 4, shelf)
    assert.ok(ids.every((id) => id.startsWith(prefix)), shelf)
  }
})

test('each recipe has its texts', () => {
  for (const r of recipes) {
    for (const key of ['name', 'formula', 'catches', 'readAs']) {
      assert.ok(typeof r[key] === 'string' && r[key].length > 0, `${r.id}.${key}`)
    }
    assert.ok(r.situations.length >= 2, `${r.id} situations`)
    for (const s of r.situations) {
      assert.ok(s.text, r.id)
      assert.ok(['alerts', 'no', 'missed'].includes(s.result), `${r.id}: ${s.result}`)
    }
    assert.ok(r.before.length >= 1 && r.before.every((b) => typeof b === 'string' && b), `${r.id} before`)
  }
})

test('every condition has the form\'s 12 keys, its config keys, and the right types', () => {
  for (const r of recipes) {
    for (const c of r.conditions) {
      const at = `${r.id} ${c.check_name}`
      assert.deepEqual(Object.keys(c).sort(), FORM_KEYS, at)
      assert.deepEqual(Object.keys(c.config).sort(), Object.keys(getDefaultConfig(c.check_type)).sort(), at)
      assert.equal(typeof c.value, 'string', at)
      assert.ok(Number.isInteger(c.condition_group), at)
      assert.equal(c.check_id, '', at)
      assert.ok(c.check_name, at)
    }
  }
})

test('conditions in one group share the group operator', () => {
  for (const r of recipes) {
    const ops = new Map()
    for (const c of r.conditions) {
      if (!ops.has(c.condition_group)) ops.set(c.condition_group, new Set())
      ops.get(c.condition_group).add(c.group_operator)
    }
    for (const [g, set] of ops) assert.equal(set.size, 1, `${r.id} group ${g}`)
  }
})

test('every condition passes the form\'s own validation', () => {
  for (const r of recipes) {
    for (const c of r.conditions) {
      const at = `${r.id} ${c.check_name}`
      assert.equal(failWindowError(c), '', at)
      assert.equal(valueError(c), '', at)
      assert.equal(c.fail_window, minFailWindow(c), at)
      assert.ok(c.interval_s >= 10 && c.interval_s <= 86400, at)
    }
  }
})

test('check names are unique within a recipe', () => {
  for (const r of recipes) {
    const names = r.conditions.map((c) => c.check_name)
    assert.equal(new Set(names).size, names.length, r.id)
  }
})

test('preferred type is used by a condition, and the PRIMARY check is the one that feeds SLA', () => {
  for (const r of recipes) assert.ok(r.conditions.some((c) => c.check_type === r.preferred), r.id)
  const expected = {
    B1: 'Ping', B2: 'Web page', B3: 'Service port', B4: 'Certificate',
    C1: 'Web page', C2: 'Web page', C3: 'Web page', C4: 'Web page',
    S1: 'Web page answers', S2: 'DNS at Cloudflare', S3: 'Remote desktop port', S4: 'Admin page hidden',
  }
  for (const r of recipes) assert.equal(primaryName(r), expected[r.id], r.id)
})

test('needs: exactly three fields are left for the person, and they are empty', () => {
  const all = recipes.flatMap((r) => r.needs.map((n) => ({ id: r.id, ...n })))
  assert.deepEqual(all.map((n) => `${n.id} ${n.condition} ${n.field}`),
    ['B3 0 config.port', 'S2 0 config.expect_value', 'S2 1 config.expect_value', 'S4 0 config.endpoint'])
  for (const r of recipes) {
    for (const n of r.needs) {
      const [a, b] = n.field.split('.')
      assert.ok([undefined, null, ''].includes(r.conditions[n.condition][a][b]), `${r.id} ${n.field} is empty`)
    }
  }
  assert.equal(byId('B3').needs[0].message, "This recipe needs the service's port here.")
  for (const n of byId('S2').needs) assert.equal(n.message, 'This recipe needs your public address here.')
  assert.equal(byId('S2').needs[0].placeholder, 'your public address')
  const s4 = byId('S4').needs[0]
  assert.equal(s4.message, "This recipe needs the admin page's path here, starting with /.")
  assert.equal(s4.pattern, '^/\\S')
  assert.ok(new RegExp(s4.pattern).test('/admin') && !new RegExp(s4.pattern).test('admin') && !new RegExp(s4.pattern).test(''))
  // no other field in any recipe is empty where the checker needs a value
  for (const r of recipes) {
    const stillEmpty = r.conditions.flatMap((c, i) => Object.entries(c.config)
      .filter(([k, v]) => v === '' && ['expect_value', 'endpoint', 'port'].includes(k))
      .map(([k]) => `${i}.config.${k}`))
    assert.deepEqual(stillEmpty, r.needs.map((n) => `${n.condition}.${n.field}`), r.id)
  }
})

test('spot settings from the spec', () => {
  const b4 = byId('B4').conditions[0]
  assert.equal(b4.check_type, 'tls_cert')
  assert.equal(b4.interval_s, 600)
  assert.equal(b4.config.warn_days, 14)
  assert.equal(b4.config.port, 443)
  assert.equal(b4.fail_count, 3)
  assert.equal(b4.fail_window, 1800)

  const s3 = byId('S3').conditions[0]
  assert.equal(s3.check_type, 'tcp')
  assert.equal(s3.config.expect_closed, true)
  assert.equal(s3.config.port, 3389)
  assert.equal(s3.field + s3.comparator + s3.value, 'statuseqdown')
  assert.equal(s3.fail_count, 2)
  assert.equal(s3.fail_window, 600)

  const canary = byId('S2').conditions.find((c) => c.check_name === 'Internet canary')
  assert.equal(canary.config.query, 'one.one.one.one')
  assert.equal(canary.config.expect_value, '1.1.1.1')
  assert.equal(canary.config.nameserver, '1.1.1.1')
  assert.equal(canary.config.record_type, 'A')
  assert.equal(canary.field + canary.comparator + canary.value, 'statusequp')
  assert.equal(canary.fail_count, 1)
  assert.equal(canary.fail_window, 0)
  assert.equal(byId('S2').conditions.find((c) => c.check_name === 'DNS at Google').config.nameserver, '8.8.8.8')
  assert.equal(byId('S2').conditions.find((c) => c.check_name === 'DNS at Cloudflare').config.nameserver, '1.1.1.1')

  const c4 = byId('C4').conditions.find((c) => c.check_name === 'Web page speed')
  assert.equal(c4.field + c4.comparator + c4.value, 'response_msgt5000')
  assert.equal(c4.fail_count, 5)
  assert.equal(c4.fail_window, 300)
  assert.equal(c4.condition_group, 1)

  const s4 = byId('S4').conditions[0]
  assert.equal(s4.config.expect_status, 404)
  const s1 = byId('S1').conditions
  assert.equal(s1[0].check_type, 'page_hash')
  assert.equal(s1[0].config.baseline_hash, '')
  assert.equal(s1[1].value, 'up')
  assert.equal(s1[1].check_type, 'http')

  // basic recipes are 3 in a row at the default interval
  for (const id of ['B1', 'B2', 'B3']) {
    const c = byId(id).conditions[0]
    assert.deepEqual([c.interval_s, c.fail_count, c.fail_window], [60, 3, 180], id)
  }
  // C2: the patient group
  const c2 = byId('C2').conditions
  assert.deepEqual(c2.map((c) => [c.check_name, c.condition_group, c.fail_count, c.fail_window]),
    [['Ping', 0, 3, 180], ['Web page', 0, 3, 180], ['Web page, patient', 1, 5, 300]])
  // C3: (Ping AND page) OR certificate
  assert.deepEqual(byId('C3').conditions.map((c) => [c.check_type, c.condition_group]),
    [['ping', 0], ['http', 0], ['tls_cert', 1]])
})

test('explainRule of every recipe, with its needs filled, has no warning', () => {
  for (const r of recipes) {
    const result = explainRule(filled(r), 'Example', 1)
    const warns = result.checks.filter((c) => c.level === 'warn').map((c) => c.text)
    assert.deepEqual(warns, [], r.id)
    assert.ok(result.groups.length >= 1, r.id)
  }
})

test('applyConditions replaces the form\'s conditions and returns the previous ones for Undo', () => {
  const old = [getDefaultCondition(0), getDefaultCondition(0), getDefaultCondition(1)]
  const form = { conditions: old, preferred_check_type: 'dns', name: 'Keep me' }
  const r = byId('C1')
  const prev = applyConditions(form, r.conditions, r.preferred, r.needs)
  assert.deepEqual(Object.keys(prev).sort(), ['conditions', 'preferred_check_type'])
  assert.equal(prev.conditions.length, 3)
  assert.deepEqual(prev.conditions, old)
  assert.equal(prev.preferred_check_type, 'dns')
  assert.equal(form.conditions.length, r.conditions.length, 'replaced, not merged')
  assert.deepEqual(form.conditions.map((c) => c.check_name), r.conditions.map((c) => c.check_name))
  assert.equal(form.preferred_check_type, 'http')
  assert.equal(form.name, 'Keep me', 'other form fields untouched')
  assert.ok(form.conditions.every((c) => c.check_id === ''))
})

test('applyConditions on an empty form returns an empty previous', () => {
  const form = { conditions: [], preferred_check_type: '' }
  const prev = applyConditions(form, byId('B1').conditions, 'ping')
  assert.deepEqual(prev, { conditions: [], preferred_check_type: '' })
  assert.equal(form.conditions.length, 1)
})

test('applyConditions blanks check_id and puts needs on the condition, not in config', () => {
  const src = clone(byId('B2').conditions)
  src[0].check_id = 'abc-123'
  const form = { conditions: [], preferred_check_type: '' }
  applyConditions(form, src, 'http', [])
  assert.equal(form.conditions[0].check_id, '')
  assert.ok(!('needs' in form.conditions[0]), 'no needs key when there is nothing to fill')

  const r = byId('S2')
  applyConditions(form, r.conditions, r.preferred, r.needs)
  const [g, cf, canary] = form.conditions
  assert.deepEqual(g.needs, [{ field: 'config.expect_value', placeholder: 'your public address',
    message: 'This recipe needs your public address here.', pattern: r.needs[0].pattern }])
  assert.equal(cf.needs.length, 1)
  assert.ok(!('needs' in canary))
  for (const c of form.conditions) assert.ok(!('needs' in c.config))
  assert.deepEqual(Object.keys(g.config).sort(), Object.keys(getDefaultConfig('dns')).sort())
})

test('Review Focus 2: editing the applied conditions never changes recipes; applying twice gives fresh values', () => {
  const before = clone(recipes)
  const r = byId('S4')
  const form = { conditions: [], preferred_check_type: '' }

  applyConditions(form, r.conditions, r.preferred, r.needs)
  const first = form.conditions
  assert.notEqual(first[0], r.conditions[0])
  assert.notEqual(first[0].config, r.conditions[0].config)
  assert.notEqual(first[0].needs, undefined)

  // the person edits the form
  first[0].config.endpoint = '/typed'
  first[0].config.expect_status = 999
  first[0].check_name = 'Renamed'
  first[0].needs[0].message = 'changed'
  first[1].config.timeout_s = 1
  first[1].fail_count = 9
  first.push(getDefaultCondition())
  form.preferred_check_type = 'tcp'
  assert.deepEqual(clone(recipes), before, 'recipes are unchanged')

  // applying again gives fresh, unedited values, none shared with the first copy
  applyConditions(form, r.conditions, r.preferred, r.needs)
  const second = form.conditions
  assert.deepEqual(second, clone(r.conditions).map((c, i) => (i === 0 ? { ...c, needs: second[0].needs } : c)))
  assert.equal(second[0].config.endpoint, '')
  assert.equal(second[0].check_name, 'Admin page hidden')
  assert.equal(second[0].needs[0].message, r.needs[0].message)
  assert.notEqual(second[0], first[0])
  assert.notEqual(second[0].config, first[0].config)
  assert.notEqual(second[0].needs, first[0].needs)
  assert.notEqual(second[0].needs[0], first[0].needs[0])
  assert.equal(form.preferred_check_type, 'http')

  // and a mutation of the second copy still leaves recipes alone
  second[1].config.endpoint = '/x'
  assert.deepEqual(clone(recipes), before)
})

test('the previous conditions returned for Undo are the form\'s own values, not the recipe\'s', () => {
  const r = byId('B1')
  const form = { conditions: [], preferred_check_type: '' }
  applyConditions(form, r.conditions, r.preferred, r.needs)
  const applied = form.conditions
  const prev = applyConditions(form, byId('B2').conditions, 'http', [])
  assert.deepEqual(prev.conditions, applied)
  assert.equal(prev.preferred_check_type, 'ping')
})

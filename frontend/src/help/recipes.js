// The twelve ready-made alert rules (spec docs/design/in_app_help.md section 8).
// Plain data. Imported only by lazy help components and tests, never by the target form
// (the form imports applyConditions.js).
//
// Each condition is in exactly the shape the target form uses. `needs` lists the fields the
// person must fill in: `condition` is the index into `conditions`, `field` is a path into
// that condition (`config.port`). The data leaves those fields empty. applyConditions
// copies each need onto its condition.
//
// Every claim in the texts is backed by the code: internal/checker/*.go (checks),
// internal/scheduler/scheduler.go (baseline capture), internal/engine/engine.go (rules).
import { getDefaultConfig } from '../conditionRules.js'

// A condition in the form's shape. `rule` is [field, comparator, value, fail_count, fail_window].
function cond(type, name, interval_s, config, rule, group = 0, op = 'AND') {
  const [field, comparator, value, fail_count, fail_window] = rule
  return {
    check_id: '',
    check_type: type,
    check_name: name,
    config: { ...getDefaultConfig(type), ...config },
    interval_s,
    field,
    comparator,
    value,
    fail_count,
    fail_window,
    condition_group: group,
    group_operator: op,
  }
}

const DOWN3 = ['status', 'eq', 'down', 3, 180]
const DOWN2 = ['status', 'eq', 'down', 2, 600]
const UP2 = ['status', 'eq', 'up', 2, 600]

const ping = (group = 0) => cond('ping', 'Ping', 60, {}, DOWN3, group)
const page = (group = 0) => cond('http', 'Web page', 60, {}, DOWN3, group)
const certificate = (group = 0) => cond('tls_cert', 'Certificate', 600, { warn_days: 14 }, ['status', 'eq', 'down', 3, 1800], group)

const OWN = "Give security recipes a target of their own, such as 'Payroll Portal security', so the alert names the threat and reaches your security team."

export const recipes = [
  {
    id: 'B1',
    shelf: 'basic',
    name: 'Reachable on the network',
    formula: 'Ping fails 3 in a row',
    catches: 'A switch, firewall or server that has dropped off the network.',
    readAs: 'Down when ping has failed 3 checks in a row: 2 to 3 minutes into an outage, with one check a minute.',
    situations: [
      { text: 'A single ping goes missing.', result: 'no' },
      { text: 'Ping fails twice, then answers.', result: 'no' },
      { text: 'Ping fails 3 checks in a row.', result: 'alerts' },
    ],
    before: [
      'Good for anything with an address: switches, firewalls, printers, badge readers.',
      'Some servers are set to ignore ping. For those, use Service port open.',
    ],
    preferred: 'ping',
    conditions: [ping()],
    needs: [],
  },
  {
    id: 'B2',
    shelf: 'basic',
    name: 'Website answers',
    formula: 'Web page fails 3 in a row',
    catches: 'A website or web app that shows errors or stops answering.',
    readAs: 'Down when the page has not answered with 200 (OK) on 3 checks in a row.',
    situations: [
      { text: 'One slow or failed page load.', result: 'no' },
      { text: 'The page shows an error 3 checks in a row.', result: 'alerts' },
      { text: 'The server stops answering.', result: 'alerts' },
    ],
    before: [
      'Bekci follows redirects, up to 10, and judges the page it ends on.',
      'For a site behind a login, check the login page: it answers without signing in.',
    ],
    preferred: 'http',
    conditions: [page()],
    needs: [],
  },
  {
    id: 'B3',
    shelf: 'basic',
    name: 'Service port open',
    formula: 'Port refuses 3 in a row',
    catches: 'A service that stops listening: mail, a database, SSH.',
    readAs: 'Down when the port has refused a connection (or not answered) on 3 checks in a row.',
    situations: [
      { text: 'One refused connection.', result: 'no' },
      { text: 'The service stops, so the port refuses 3 checks in a row.', result: 'alerts' },
    ],
    before: [
      'It proves something is listening, not that the service works. Where there is a web page, Website answers tells you more.',
      'Common ports: 25 mail, 22 SSH, 3306 and 5432 databases.',
    ],
    preferred: 'tcp',
    conditions: [cond('tcp', 'Service port', 60, { port: '' }, DOWN3)],
    needs: [{
      condition: 0,
      field: 'config.port',
      placeholder: "the service's port",
      message: "This recipe needs the service's port here.",
      pattern: null,
    }],
  },
  {
    id: 'B4',
    shelf: 'basic',
    name: 'Certificate renewal',
    formula: 'Under 14 days, 3 checks in a row',
    catches: 'A certificate about to expire, with two weeks left to renew.',
    readAs: 'Down when the certificate has fewer than 14 days left on 3 checks in a row, checked every 10 minutes.',
    situations: [
      { text: '45 days left.', result: 'no' },
      { text: 'The site is down for a quarter of an hour.', result: 'no' },
      { text: '13 days left.', result: 'alerts' },
    ],
    before: [
      "The check also fails when the site doesn't answer. Three checks 10 minutes apart stop a short outage passing for an expiring certificate.",
      'An expiring certificate stays expiring, so waiting half an hour costs nothing.',
    ],
    preferred: 'tls_cert',
    conditions: [certificate()],
    needs: [],
  },
  {
    id: 'C1',
    shelf: 'combined',
    name: 'Server down, confirmed',
    formula: 'Ping AND web page',
    catches: 'A server that is really down, confirmed by two different tests.',
    readAs: 'Down when ping and the web page have both failed 3 checks in a row.',
    situations: [
      { text: 'Ping drops, the page still loads.', result: 'no' },
      { text: 'Both fail 3 in a row.', result: 'alerts' },
      { text: 'The web app fails while the server still answers ping.', result: 'missed' },
    ],
    before: [
      "It won't catch the web app failing on a server that still answers ping. Fast when sure catches both.",
    ],
    preferred: 'http',
    conditions: [ping(), page()],
    needs: [],
  },
  {
    id: 'C2',
    shelf: 'combined',
    name: 'Fast when sure, patient when not',
    formula: 'Both fail: 3. Page alone: 5',
    catches: 'Every outage: quickly when two tests agree, a little later when only one does.',
    readAs: 'Down when ping and the web page have both failed 3 checks in a row, or when a second check of the web page has failed 5 checks in a row.',
    situations: [
      { text: 'Ping drops, the page loads.', result: 'no' },
      { text: 'The server goes off.', result: 'alerts' },
      { text: 'The web app fails, the server answers ping: alerts after 5 checks.', result: 'alerts' },
      { text: 'One failed page load.', result: 'no' },
    ],
    before: [
      'Two tests agreeing is strong evidence, so it can alert sooner.',
      'Group 2 adds a second web check on the same page, so the page is loaded twice a minute.',
    ],
    preferred: 'http',
    conditions: [
      ping(0),
      page(0),
      cond('http', 'Web page, patient', 60, {}, ['status', 'eq', 'down', 5, 300], 1),
    ],
    needs: [],
  },
  {
    id: 'C3',
    shelf: 'combined',
    name: 'Web server and its certificate',
    formula: '(Ping AND page) OR certificate',
    catches: 'The server going down, or its certificate needing renewal.',
    readAs: 'Down when ping and the web page have both failed 3 checks in a row, or when the certificate has fewer than 14 days left on 3 checks in a row.',
    situations: [
      { text: 'Ping drops, the page loads.', result: 'no' },
      { text: 'The server is unreachable.', result: 'alerts' },
      { text: '13 days left on the certificate.', result: 'alerts' },
    ],
    before: [
      'The two groups are alternatives: either one alerts on its own.',
    ],
    preferred: 'http',
    conditions: [ping(0), page(0), certificate(1)],
    needs: [],
  },
  {
    id: 'C4',
    shelf: 'combined',
    name: 'Down or too slow',
    formula: 'Page fails OR slower than 5 s',
    catches: 'No answer at all, or answers too slow to use.',
    readAs: 'Down when the web page has failed 3 checks in a row, or when a second check of it has taken longer than 5000 ms on 5 checks in a row.',
    situations: [
      { text: 'One slow load.', result: 'no' },
      { text: 'Slower than 5 seconds for 5 checks.', result: 'alerts' },
      { text: 'An error 3 checks in a row.', result: 'alerts' },
    ],
    before: [
      "Look at the page's usual response time first and set the limit well above it.",
      'Group 2 adds a second web check on the same page.',
    ],
    preferred: 'http',
    conditions: [
      page(0),
      cond('http', 'Web page speed', 60, {}, ['response_ms', 'gt', '5000', 5, 300], 1),
    ],
    needs: [],
  },
  {
    id: 'S1',
    shelf: 'security',
    name: 'Defacement, without outage noise',
    formula: 'Content changed AND site answering',
    catches: 'Someone changing what your page says.',
    readAs: "Down when the page's content has changed on 2 checks in a row while the site is answering normally.",
    situations: [
      { text: 'The site goes down: an outage, not tampering.', result: 'no' },
      { text: 'The content changes while the site answers.', result: 'alerts' },
      { text: 'Nothing changes.', result: 'no' },
    ],
    before: [
      'Pick a page that only changes when you publish. A page with the date, news or ads changes on every load.',
      'The first check records the page as the baseline. After a planned change, clear Baseline Hash and save: Bekci records the new page as the baseline.',
      'Why the pairing: Page Hash reports down for a changed page and also for an unreachable or non-2xx page, so on its own it cannot tell defacement from an outage.',
      OWN,
    ],
    preferred: 'http',
    conditions: [
      cond('page_hash', 'Page content', 300, {}, DOWN2, 0),
      cond('http', 'Web page answers', 300, {}, UP2, 0),
    ],
    needs: [],
  },
  {
    id: 'S2',
    shelf: 'security',
    name: 'DNS hijack',
    formula: 'Two resolvers wrong, internet fine',
    catches: 'Your name pointing somewhere else, as the outside world sees it.',
    readAs: 'Down when two public resolvers both give an address other than yours (or fail to answer) on 2 checks in a row, while Bekci can reach the internet.',
    situations: [
      { text: 'One resolver has a hiccup.', result: 'no' },
      { text: "Bekci's own internet goes down: the canary fails too.", result: 'no' },
      { text: 'Both resolvers give a new address.', result: 'alerts' },
    ],
    before: [
      'Only for names with a fixed address. A CDN or cloud load balancer changes address and would alert.',
      "The target's address must be a name, not an IP address: the lookup asks for the target's host name.",
      'The third test is a canary: it looks up a name whose answer never changes, to prove Bekci itself is online.',
      'Bekci must be allowed to query public DNS. If its network blocks that, the canary never passes and the rule can never fire, so check the canary shows up after saving.',
      OWN,
    ],
    preferred: 'dns',
    conditions: [
      cond('dns', 'DNS at Google', 300, { nameserver: '8.8.8.8', expect_value: '' }, DOWN2, 0),
      cond('dns', 'DNS at Cloudflare', 300, { nameserver: '1.1.1.1', expect_value: '' }, DOWN2, 0),
      cond('dns', 'Internet canary', 300,
        { query: 'one.one.one.one', nameserver: '1.1.1.1', expect_value: '1.1.1.1' }, ['status', 'eq', 'up', 1, 0], 0),
    ],
    needs: [0, 1].map((condition) => ({
      condition,
      field: 'config.expect_value',
      placeholder: 'your public address',
      message: 'This recipe needs your public address here.',
      pattern: null,
    })),
  },
  {
    id: 'S3',
    shelf: 'security',
    name: 'A port that must stay closed',
    formula: 'Alert when the port opens',
    catches: 'Remote desktop, a database or file sharing becoming reachable, for example after a firewall change.',
    readAs: 'Down when port 3389 accepts a connection on 2 checks in a row. The check has Port must stay closed ticked, so a closed port is up and an open one down.',
    situations: [
      { text: 'The port stays closed.', result: 'no' },
      { text: "The machine is switched off: a port that doesn't answer counts as closed.", result: 'no' },
      { text: 'The port starts accepting connections.', result: 'alerts' },
    ],
    before: [
      'It tests from where Bekci runs, so it shows what that network can reach.',
      'Ports worth watching: 3389 remote desktop, 22 SSH, 445 file sharing, 3306 and 5432 databases.',
      'Uptime and SLA read 100% while the port stays closed.',
      OWN,
    ],
    preferred: 'tcp',
    conditions: [cond('tcp', 'Remote desktop port', 300, { port: 3389, expect_closed: true }, DOWN2)],
    needs: [],
  },
  {
    id: 'S4',
    shelf: 'security',
    name: 'Admin page that must stay hidden',
    formula: 'Admin page answers AND site up',
    catches: 'An admin or setup page becoming reachable from where Bekci runs.',
    readAs: 'Down when the admin page stops returning 404 on 2 checks in a row while the site is answering normally.',
    situations: [
      { text: 'The site goes down: an outage, not exposure.', result: 'no' },
      { text: 'The admin page shows a page or a login.', result: 'alerts' },
      { text: 'It still returns 404.', result: 'no' },
    ],
    before: [
      'Bekci follows redirects, so a redirect to a login page counts as reachable.',
      'Set Expected Status to what the page returns today: usually 404 or 403.',
      'The admin path must start with "/". The check adds the path straight after the host, so an empty path becomes "/" (the home page, which answers and so false-alarms) and a path without the "/" gives a broken address.',
      OWN,
    ],
    preferred: 'http',
    conditions: [
      cond('http', 'Admin page hidden', 300, { endpoint: '', expect_status: 404 }, DOWN2, 0),
      cond('http', 'Web page answers', 300, {}, UP2, 0),
    ],
    needs: [{
      condition: 0,
      field: 'config.endpoint',
      placeholder: "the admin page's path, such as /admin",
      message: "This recipe needs the admin page's path here, starting with /.",
      pattern: '^/\\S',
    }],
  },
]

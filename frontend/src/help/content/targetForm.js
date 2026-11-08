// Help for the target form: New, Edit and Clone (design §7.2). The alert rule is
// built here, so its notes get the most care. Per-condition and per-group
// controls have a mark on every instance (`each`); the Fail Count and Time Window
// notes are written from that condition's own numbers, which the form gives as
// data-help-args { fail_count, interval_s }.
//
// Facts: fields and defaults TargetEditModal.vue and conditionRules.js; window
// bounds internal/api/target_handlers.go normalizeFailWindow; interval clamp
// target_handlers.go (10..86400); Fail Count runs internal/engine/engine.go
// evaluateCondition; groups engine.go evaluateRule; checkers internal/checker/*.go
// (timeout below 1 dropped: checker.go dropInvalidTimeout); next run one interval
// after the last ends: internal/scheduler/scheduler.go runAndReschedule; baseline
// capture scheduler.go runCheck; SNMP credentials injected from Settings in
// runCheck; Run Now ignores Enabled: internal/api/check_handlers.go
// handleRunCheckNow; cooldown and reminders internal/alerter/alerter.go, defaults
// internal/store/store.go; checks removed on Save internal/store/targets.go
// UpdateTarget (results cascade).
import { failCountTimeline } from '../explainRule.js'
import { maxFailWindow, minFailWindow } from '../../conditionRules.js'

export const targetForm = {
  summary: {
    name: 'The target form',
    text: 'sets what Bekci watches, how it tests it, and exactly when it counts as down. The alert rule below is what keeps false alarms out.',
  },
  items: {
    name: {
      title: 'Name',
      text: ['The name shown everywhere: lists, alerts, reports.'],
      box: 'label',
    },
    host: {
      title: 'Host',
      text: [
        'The address Bekci tests: a hostname or IP, optionally followed by a path (www.example.com/health). HTTP and Page Hash checks ask for that path, with their Endpoint after it; every other check uses only the name. A DNS check given its own Query asks for that instead.',
        "Leave out http:// or https:// and the port: each check has its own Scheme and Port. Adding a path to a Host with a Page Hash check changes the page it reads, so clear that check's Baseline Hash to capture the new page.",
      ],
      box: 'label',
    },
    category: {
      title: 'Category',
      text: ["Which group the target belongs to. The Targets page and the Dashboard filter by it, the SLA page groups targets by it, and it sets the target's SLA goal (admins set one per category in Settings › SLA)."],
      box: 'label',
    },
    preferred: {
      title: 'Preferred Check',
      text: ["Appears once there is more than one condition. A check of this type feeds the target's SLA figure and the uptime on the Dashboard; if there are several of that type, the first by name counts."],
      box: 'label',
    },
    notes: {
      title: 'Notes and Contacts',
      text: ['Free text kept with the target, for whoever handles it.'],
      box: 'label',
    },
    filters: {
      title: 'Project, Location and Tags',
      text: ['Labels for finding the target later: they become filters on the Targets and Search pages.'],
      box: 'label',
    },
    enabled: {
      title: 'Enabled',
      text: ['Untick to switch the target off: its checks stop running on schedule until it is ticked again, and it is hidden from the Dashboard and SLA pages. A Run Now still runs them.'],
      box: 'label',
    },
    groups: {
      title: 'Condition Groups',
      text: ['The alert rule. Each group is one way for this target to count as down.'],
      link: 'lesson',
    },
    addgroup: {
      title: '+ Add Group or Condition',
      text: ['Adds a new group, joined to the others by OR: one more way to count as down.'],
      place: 'left',
    },
    group: {
      title: 'Group',
      text: ['A group holds one or more conditions, joined by the AND / OR switch next to it.'],
      place: 'left',
      each: true,
    },
    andor: {
      title: 'AND / OR',
      text: ["How this group's conditions combine."],
      rows: [
        { control: { label: 'AND', className: 'op-btn active' }, text: "All must be failing at the same time. One flaky test can't raise an alarm on its own." },
        { control: { label: 'OR', className: 'op-btn active' }, text: 'Any one failing is enough. Separate groups do the same job and are easier to read.' },
      ],
      each: true,
    },
    remove: {
      title: 'Remove',
      text: ['Removes the condition or the whole group from the form. On Save, a removed check is deleted together with its history.'],
      place: 'left',
      each: true,
    },
    type: {
      title: 'Check Type',
      text: ["What to test: ping, a web page (HTTP/HTTPS), a port that should be open or closed (TCP), DNS, a page's content (Page Hash), a certificate (TLS) or SNMP. The fields below change with the type. Fixed once saved."],
      box: 'label',
      each: true,
    },
    int: {
      title: 'Interval',
      text: [
        'Seconds between checks. 60 is once a minute. The next check starts one interval after the previous one finishes. The server keeps it within 10 to 86400.',
        'A shorter interval notices sooner; Fail Count decides when that becomes an alert.',
      ],
      box: 'label',
      each: true,
    },
    timeout: {
      title: 'Timeout',
      text: ["How long to wait for an answer. A value below 1 falls back to the check's default."],
      box: 'label',
      each: true,
    },
    closed: {
      title: 'Port must stay closed',
      text: [
        'For a port that must never open, such as remote desktop. A closed port, or one that doesn\'t answer, counts as up and a connection as down, so the usual Status equals down alerts when it opens and the uptime stays at 100% while it is closed.',
        'A host that is switched off also reads up on this check: add a ping check if that matters.',
      ],
      box: 'label',
      each: true,
    },
    expect: {
      title: 'Expected Status',
      text: ['Any other status counts as down. Bekci follows redirects and judges the page it ends on.'],
      box: 'label',
      each: true,
    },
    skiptls: {
      title: 'Skip TLS Verify',
      text: ['Accept a certificate that would normally be refused (self-signed, wrong name). For internal systems only.'],
      box: 'label',
      each: true,
    },
    pingcount: {
      title: 'Ping Count',
      text: ['Pings sent per check. The check fails only if none come back. Response (ms) is their average round trip. More pings make Packet loss finer (steps of 100 ÷ count) and add about a second each.'],
      box: 'label',
      each: true,
    },
    dnsexpect: {
      title: 'Expected Value',
      text: ['If filled in, the check fails unless this value is among the answers. Empty: it fails only when the lookup fails.'],
      box: 'label',
      each: true,
    },
    dnsns: {
      title: 'Nameserver',
      text: ["Ask this server instead of the system's resolver, for example 8.8.8.8."],
      box: 'label',
      each: true,
    },
    baseline: {
      title: 'Baseline Hash',
      text: ['Empty: Bekci records the page the first time it reads it (a page that is down or not 2xx records nothing), and compares every later check with that. Clear it after a planned change to record the new page.'],
      box: 'label',
      each: true,
    },
    warn: {
      title: 'Warning Days',
      text: [
        'The certificate check reports down once fewer days than this are left. It also reports down if the certificate can\'t be checked.',
        'It reads the expiry date only: a self-signed or wrong-name certificate still counts as up.',
      ],
      box: 'label',
      each: true,
    },
    snmp: {
      title: 'SNMP credentials',
      text: ['Credentials are set once for all targets, by an admin, in Settings.'],
      box: 'content',
      each: true,
    },
    when: {
      title: 'Alert when...',
      text: [
        'Describes a failing result. Usually Status equals down. To catch slowness, Response (ms) greater than, say, 3000.',
        'To catch a lossy link on a ping check, Packet loss (%) greater than 30 with Fail Count 3; with 3 pings it moves in steps of 33.3%, so greater than 30 already means one lost ping (the hint under the row says this for the actual Ping Count). Picking Response (ms) or Packet loss switches to greater than and asks for a number.',
        'To alert when a port opens that should stay closed, tick Port must stay closed on the TCP check and keep Status equals down.',
      ],
      box: 'content',
      each: true,
    },
    fc: {
      title: 'Fail Count',
      text: ['How many results in a row must fail before this condition counts; one good result starts the count again.'],
      // Written with the figures Bekci uses (an interval under 10 seconds runs every 10)
      timeline: args => {
        const timeline = failCountTimeline(args.fail_count, args.interval_s)
        return { ...timeline, caption: `With this condition's numbers: a check every ${timeline.interval} seconds, Fail Count ${timeline.failCount}` }
      },
      tip: 'Rule of thumb: 3, for almost every check. A real problem keeps failing, so waiting costs a few minutes and filters out blips. Rule recipes has ready-made rules.',
      box: 'label',
      each: true,
    },
    tw: {
      title: 'Time Window',
      text: args => {
        const first = 'The failures in a row must all fall inside this many seconds.'
        const least = minFailWindow(args)
        if (!least) {
          return [first, `Bekci fills it in once Fail Count is above 1: at least Fail Count × Interval, at most ${maxFailWindow}. With Fail Count 1 there is no window: only the latest result counts.`]
        }
        // The sum with the figures Bekci uses, the same as the Fail Count note's
        const { failCount, interval } = failCountTimeline(args.fail_count, args.interval_s)
        const sum = failCount * interval === least ? `${failCount} × ${interval} = ${least}` : `${least}`
        const lines = [first, `Bekci fills it in once Fail Count is above 1: at least Fail Count × Interval (here ${sum}), at most ${maxFailWindow}.`]
        if (least > maxFailWindow) lines.push(`${least} is more than ${maxFailWindow}, so Save is refused until Fail Count or Interval is lower.`)
        return lines
      },
      box: 'label',
      each: true,
    },
    addcond: {
      title: '+ Add condition to group',
      text: ["Adds a test to this group, joined by the group's AND / OR."],
      each: true,
    },
    or: {
      title: 'OR between groups',
      text: ['Groups are always joined by OR. Each group is a separate way to count as down, so adding a group adds ways to be alerted; it never confirms another group.'],
      tip: 'Want two tests to agree? Put them in the same group with AND.',
      each: true,
    },
    rcp: {
      title: 'Alert Recipients',
      text: ['The people messaged when this target goes down and when it recovers. A second change within the cooldown (30 minutes by default) is not messaged; while it stays down they get a reminder every hour by default.'],
      tip: 'A new or cloned target starts with nobody ticked, so it sends no email or Signal until you tick someone. The webhook, if on, needs no recipients.',
    },
    save: {
      title: 'Save and Cancel',
      text: ['Nothing is saved until you press Save, which applies everything in the form; Cancel discards the changes.'],
      box: 'content',
    },
  },
}

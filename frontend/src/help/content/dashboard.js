// Help for the Dashboard (design §7.4).
export const dashboard = {
  summary: {
    name: 'Dashboard',
    text: "shows every switched-on target's state right now, problems first, with 90 days of daily uptime and the last 4 hours. Click a target to see each of its checks.",
  },
  items: {
    updated: {
      title: 'Updated',
      text: ['States and figures refresh every 30 seconds. The bars do not: reload the page for the latest.'],
      place: 'left',
    },
    cats: {
      title: 'Categories',
      text: ["Show one category. A red dot on a category you aren't viewing means at least one target in it is down."],
      box: 'content',
    },
    row: {
      title: 'Target rows',
      text: [
        'Click to unfold its checks; one target opens at a time.',
        'Order: down targets first; then targets under their SLA goal; then the rest; paused targets always last. Within each group, the lowest uptime of any of its checks first.',
      ],
    },
    updown: {
      title: 'UP / DOWN',
      text: ["Whether the target counts as down right now, by its alert rule (Fail Count, AND/OR and all). One red check doesn't make it DOWN if the rule needs more."],
      place: 'corner',
    },
    sla: {
      title: 'HEALTHY / UNHEALTHY',
      text: [
        "The SLA verdict: the primary check's uptime over the last 90 days against the category's goal (Settings › SLA; 99.9% if none is set; none shown if the goal is 0).",
        'It is separate from UP/DOWN: a target can be UP and UNHEALTHY.',
      ],
      place: 'corner',
    },
    pct: {
      title: '90-day uptime',
      text: [
        'Share of all runs of the primary check that were up, over the last 90 days. Every failed run counts, even one that never caused an alert, because Fail Count and AND/OR only decide alerts.',
        "Colours are fixed: green from 99.9, yellow from 95, orange from 80, red below; they don't follow the SLA goal.",
      ],
      place: 'corner',
    },
    type: {
      title: 'Check type and response time',
      text: ["The primary check's type and the response time of its latest run."],
      place: 'corner',
    },
    bars90: {
      title: '90-day strip',
      text: [
        "One bar per day. Hover for that day's uptime and number of runs.",
        "Grey means no data (its tooltip shows -1.0%). Days with no data, such as paused days, don't count against uptime.",
      ],
      place: 'corner',
    },
    bars4h: {
      title: '4-hour strip',
      text: ['The last 4 hours in 5-minute slots, each showing the last run in that slot, so a failure earlier in the slot can be hidden.'],
      place: 'corner',
    },
    prim: {
      title: 'PRIMARY',
      text: ['The check that drives the header, the uptime and the SLA badge. With several checks of the preferred type, only the first by name is marked.'],
      place: 'corner',
    },
    check: {
      title: 'Check rows',
      text: ["Click to see that check's latest message. Its dot is its latest run, not the target's state."],
      place: 'left',
    },
    pages: {
      title: 'Pages',
      text: ['10 targets per page.'],
      box: 'content',
    },
  },
}

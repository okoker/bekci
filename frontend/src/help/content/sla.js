// Help for SLA (design §7.6). Facts: SlaView.vue:47-79, 119, 127-138; sla_handlers.go:65-112, 150-162;
// store/targets.go:815-822.
export const sla = {
  summary: {
    name: 'SLA',
    text: "shows each category's daily uptime over the last 90 days, one line per host, against the category's goal (the dashed line). Click a chart to enlarge it.",
  },
  items: {
    hosts: {
      title: 'Hosts Monitored',
      text: ['Switched-on targets with at least one check, paused ones included.'],
      place: 'corner',
    },
    notmeet: {
      title: 'Not Meeting SLA',
      text: ["Hosts whose 90-day uptime is under their category's goal: up runs divided by all runs, the same figure as the Dashboard's. Hosts without data and categories with goal 0 are left out."],
      place: 'corner',
    },
    avg: {
      title: 'Averages',
      text: ["The All card and the category cards show averages of each host's 90-day uptime. Every host counts equally, however often it is checked. Hosts without data are left out."],
      place: 'corner',
    },
    planned: {
      title: 'Planned Work',
      text: ['Pauses started this calendar month, and how many hosts they covered.'],
      place: 'corner',
    },
    chart: {
      title: 'Category charts',
      text: [
        'One line per host. The dashed line is the category\'s goal, shown only when the goal is above 0. Hover to pick out a host; click to enlarge.',
        "Gaps are drawn as a continuous line, and the scale doesn't start at 0.",
      ],
      place: 'corner',
    },
    zoom: {
      title: 'Enlarged chart',
      text: ['Scroll to zoom, drag to pan; Reset zoom appears once you have zoomed. Esc, ✕ or a click outside closes it.'],
      place: 'corner',
    },
  },
}

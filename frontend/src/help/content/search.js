// Help for Search (design §7.5). Facts: SearchView.vue (localStorage saved searches, matching on name and host).
export const search = {
  summary: {
    name: 'Search',
    text: 'finds targets by name or host and narrows them by project, location and tags. Open a result to see its notes, contacts, recipients and checks, and act on it.',
  },
  items: {
    q: {
      title: 'Search box',
      text: ['Matches any part of the name or host, ignoring case. An IP address is found only if it is the host.'],
      place: 'corner',
    },
    filters: {
      title: 'Filters',
      text: ['Narrow the results to one project, one site, or targets carrying every tag you pick.'],
      place: 'corner',
    },
    save: {
      title: '+ Save',
      text: [
        'Appears once you type a search or pick a filter. Saves the current search under a name.',
        "Saved searches live in this browser only: they don't follow your account, and anyone using this browser sees them.",
      ],
    },
    saved: {
      title: 'Saved searches',
      text: ['Click one to replace the current filters with it. × deletes it straight away.'],
    },
    state: {
      title: 'State',
      text: ['Whether the target is up, judged by its alert rule.'],
      rows: [
        { control: { label: 'healthy', className: 'badge badge-active' }, text: 'No part of its rule is met. A new target starts healthy until its checks report.' },
        { control: { label: 'unhealthy', className: 'badge badge-suspended' }, text: 'Its rule is met, so it counts as down and its recipients are alerted.' },
        { control: { label: 'paused', className: 'badge badge-paused' }, text: 'Switched off for maintenance.' },
        { control: { label: '—', className: 'text-muted' }, text: 'No checks, so there is no rule to judge it by.' },
      ],
      link: 'lesson',
    },
    acts: {
      title: 'Actions',
      text: ['Pause or Unpause, Run Now, Edit, Clone and Delete work as on the Targets page (operators and admins).'],
      rows: [
        { control: { label: 'Pause', className: 'btn btn-sm btn-pause' }, text: 'Stops its scheduled checks for maintenance. Bekci asks first.' },
        { control: { label: 'Unpause', className: 'btn btn-sm btn-unpause' }, text: 'Starts them again and runs every check at once. Bekci asks first.' },
        { control: { label: 'Delete', className: 'btn btn-sm' }, text: 'Removes the target, its checks and their whole history. Bekci asks first.' },
      ],
      tip: 'The question opens next to the button you pressed.',
    },
    row: {
      title: 'Results',
      text: [
        'Click to unfold notes, contacts, recipients, dates and checks.',
        'The list includes switched-off targets without marking them; the Targets page shows which.',
      ],
    },
    strip: {
      title: 'Last 4 hours',
      text: ['As on the Dashboard, one strip per check; each loads when the target is unfolded.'],
    },
  },
}

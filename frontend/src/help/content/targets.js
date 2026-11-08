// Help for Targets (design §7.3). Facts: the helper inventory of TargetsView.vue;
// Run Now internal/api/check_handlers.go:17-31; pause audit target_handlers.go:545, 565-576.
export const targets = {
  summary: {
    name: 'Targets',
    text: 'lists every system Bekci watches, including switched-off ones, and whether each is up right now. Here you add them, decide when each counts as down, and pause, test or remove them.',
  },
  items: {
    add: {
      title: '+ Target',
      text: ['Adds a new system to watch. You choose what to test and exactly when it counts as down.'],
      tip: "Operators and admins only; viewers don't see this button.",
      place: 'left',
    },
    cats: {
      title: 'Categories',
      text: ['Show one category. The number is how many targets it holds, whatever the other filters.'],
      tip: 'Admins keep the category list in Settings › Tags.',
      box: 'content',
    },
    filt: {
      title: 'Project, Location and Tags',
      text: ['Narrow the list to one project, one site, or targets carrying every tag you pick. They combine with the category.'],
      box: 'content',
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
      linkLabel: 'How alert rules work',
    },
    cond: {
      title: 'Conditions',
      text: ['How many checks this target has. Edit shows the rule that combines them.'],
      place: 'corner',
    },
    en: {
      title: 'Enabled',
      text: ['"no" (the row is faded and marked DISABLED) means the target is switched off in its settings. Nothing runs on schedule, and it is hidden from the Dashboard and SLA pages.'],
      place: 'corner',
    },
    acts: {
      title: 'Actions',
      text: ['What you can do to one target (operators and admins).'],
      rows: [
        { control: { label: 'Pause', className: 'btn btn-sm btn-pause' }, text: "Stops its scheduled checks for maintenance, so nothing alerts on schedule (a Run Now still can) and its SLA isn't affected. Bekci asks first; recorded in the Audit Log." },
        { control: { label: 'Unpause', className: 'btn btn-sm btn-unpause' }, text: 'Starts them again and runs every check at once. Bekci asks first.' },
        { control: { label: 'Run Now', className: 'btn btn-sm' }, text: 'Runs every check right away instead of waiting its turn. Each run is recorded in the Audit Log; it runs even on a paused target, and those results count.' },
        { control: { label: 'Edit', className: 'btn btn-sm' }, text: 'Change the target, its checks or its rule.' },
        { control: { label: 'Clone', className: 'btn btn-sm' }, text: 'Start a new target with the same checks and rule; you fill in the host and choose recipients.' },
        { control: { label: 'Delete', className: 'btn btn-sm btn-danger' }, text: 'Removes the target, its checks and their whole history, SLA included. Bekci asks first.' },
      ],
    },
    row: {
      title: 'Target rows',
      text: ['Click a row to unfold the checks behind it: what each tests, how often, and which is PRIMARY.'],
    },
    prim: {
      title: 'PRIMARY',
      text: [
        'When a target runs several kinds of check, the check marked PRIMARY feeds its SLA figure and the uptime on the Dashboard.',
        'With several checks of that type, the first by name is the primary one and only it is marked.',
      ],
      tip: 'Chosen in the target form under Preferred Check, which appears once a target has more than one condition.',
      place: 'corner',
    },
    pages: {
      title: 'Pages',
      text: ['50 targets per page. Changing a filter goes back to page 1.'],
      box: 'content',
    },
    guide: {
      title: 'Target creation guide',
      text: ['A short cheat sheet for building targets, kept on this page. The help covers the same ground with more examples.'],
    },
  },
}

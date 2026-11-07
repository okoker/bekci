// Help for the header, explained on every page after the page's own notes
// (design §7.1). Its page id is HEADER_PAGE in ../anchors.js; it has no summary
// of its own, since the bar always names the page underneath.
export const header = {
  items: {
    dots: {
      title: 'Server health',
      text: ['Health of the Bekci server itself, not of your targets. Click for details.'],
      rows: [
        { control: { label: 'Net', className: 'health-row dot-green' },
          text: 'Green when the server can ping 1.1.1.1. A network that blocks outbound ping keeps it red even while monitoring works.' },
        { control: { label: 'Disk', className: 'health-row dot-green' },
          text: 'Free space: green above 20%, yellow above 10%, red at 10% or less.' },
        { control: { label: 'Load', className: 'health-row dot-green' },
          text: 'One-minute load: green below the number of cores, yellow below twice that, red above.' },
        { control: { label: 'Scheduler', className: 'health-row dot-green' },
          text: 'Green when its last tick was under 2 minutes ago; yellow "Starting" before its first tick; red "stale" means checks have stopped running.' },
      ],
      tip: 'Grey means no data: the figure could not be read. Net, Disk and Load can be up to about 2.5 minutes old.',
      place: 'corner',
    },
    version: {
      title: 'Version',
      text: ['The Bekci version running now.'],
      place: 'corner',
    },
    user: {
      title: 'Your account',
      text: ['Your account: Profile (alert email and phone, password) and Logout.'],
      place: 'corner',
    },
    banner: {
      title: 'Monitoring engine not responding',
      text: ['Checks have stopped running, so what you see may be out of date. Tell an admin.'],
      box: 'content',
    },
  },
}

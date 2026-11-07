// Where in-app help attaches to the page (design in_app_help.md §4.3).
//
// A view marks each documented control with data-help="<page>.<key>", a static
// string; the note for it is the item `key` of page `page` in content/. The
// header's controls use the page id below and are explained on every page.
//
// The help layer looks for marks inside the scope on screen: the last element,
// in document order, that carries data-help-scope. The layout names itself, and
// the target form's overlay, which sits inside it, names its own and wins while
// it is open.
//
// Where a view renders documented controls from a list, it takes their keys
// from a map exported here as <page>Anchors: { the view's own name: key }.
//
// Views may import this module, so it holds plain data only: no imports, no
// code that reads the page. That lives in the help layer, which loads on first use.

export const HEADER_PAGE = 'header'

// Settings › General draws its fields from the settings it loads: the key of
// each one's note, by setting name.
export const settingsGeneralAnchors = {
  session_timeout_hours: 'session',
  history_days: 'history',
  audit_retention_days: 'auditret',
  backup_max_copies: 'copies',
  soc_public: 'soc',
  api_rate_limit_per_min: 'rate',
}

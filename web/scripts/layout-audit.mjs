/**
 * The layout guard: does the interface stay reachable on a phone?
 *
 * It is a script and not a vitest case because it needs the whole product --
 * the daemon serving the embedded bundle, a real session, and every route as an
 * operator actually reaches it. A component rendered in isolation cannot answer
 * this: what pushed seventeen controls off the side of /settings was the shell
 * around them, and no test of a component contains its shell.
 *
 * WHAT IT ASSERTS: two things, and they are different failures. Nothing is
 * clipped, at both widths. And at the phone width every control a person taps
 * is at least 44px -- reach is not the same as being able to hit it, and the
 * first version of this guard measured only the first, which left ten controls
 * between 24 and 43px held by nothing but a number in a commit message.
 *
 * On being clipped: An element whose right edge is past the
 * viewport is a finding only when no scrollable ancestor can bring it back --
 * a wide table inside `overflow-x-auto` is reachable by swiping, which is ugly
 * and not broken. An element with nothing to scroll is simply gone, and that is
 * how an operator lost the "New account" form and the button that changes a
 * password.
 *
 * WHAT IT MEASURES: every route in web/scripts/layout-routes.json -- the
 * router's own leaves, held equal to it by src/layoutRoutes.test.ts, which runs
 * first -- and then the states that only appear after a tap (OPENERS): a
 * dialog, a menu, a list. Those are an explicit list because the alternative,
 * clicking every button, would press the destructive actions and would not
 * open the same thing twice. An opener is measured scoped to what opened,
 * after its animation, and must close again; one that cannot be opened, holds
 * no control, or does not close is a failure, never a skip.
 *
 * WHY IT WATCHES ITS OWN REQUESTS: it runs on the machine production runs on.
 * It only ever talks to the daemon it started in a temporary directory, and an
 * opener that submits a form is stopped by the server's sudo gate -- but that
 * is shown, not assumed: every non-GET request an opener sends is printed, and
 * one that carried an action out (EXECUTED) fails the run.
 *
 * WHY IT IS IN THE CHAIN AND NOT OPT-IN: ledger entries 5 and 64 record a drift
 * watcher that existed, was never scheduled, and therefore never watched
 * anything. A guard nobody runs is a comment.
 *
 * The widths are the operator's decision of 2026-09-17: a phone and a desk, the
 * two sides of the `md` breakpoint.
 *
 * LAYOUT_DUMP=<file> (D-08, "the desk unchanged, measured once"): the run also
 * writes, at the desk width, one JSON line per control on every route and inside
 * every opener -- route, opener, index in DOM order within the scope, tag, name,
 * and the x, y, width, height of the element's own box. Phase 14 raises tap
 * targets below `md` only; a dump taken before the first class change and one
 * after the last, judged by scripts/layout-dump-compare.mjs, is what shows the
 * desk kept its compact sizes rather than a reviewer's reading of the prefixes.
 * It is a one-off measurement, not a baseline: the path is refused inside the
 * repository, because a checked-in baseline is regenerated the first time it
 * disagrees and from then on measures nothing. Without LAYOUT_DUMP nothing of
 * this runs and the output is what it always was.
 */
import { spawn, spawnSync } from 'node:child_process'
import { appendFileSync, existsSync, readFileSync, realpathSync, writeFileSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { basename, dirname, isAbsolute, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

/**
 * What the screens are rendered with.
 *
 * Ledger 149: until this existed the guard started a daemon with an empty data
 * directory, so every screen that shows rows showed none. /kubernetes measured
 * "3 controls" for a page carrying a dozen, and the tables the operator
 * photographed as unusable were never rendered at all. A guard that only ever
 * sees the empty state passes everything that breaks when there is data --
 * which is most of what an operator looks at.
 *
 * Only GET is answered from here, so setup and login still go to the real
 * daemon: the shell, the session and the router are the product's own. What is
 * replaced is the data, and web/src/fixtures.test.ts holds it to the same zod
 * schemas the product parses real answers with, so a fixture cannot quietly
 * drift into a shape the app rejects and leave this measuring an error page.
 */
const FIXTURES = JSON.parse(
  readFileSync(fileURLToPath(new URL('../fixtures/demo.json', import.meta.url)), 'utf8'),
)

/**
 * /host as it is with the helper installed and nothing placed: the demo host
 * with only its `actions` replaced, served to the two /host openers alone.
 *
 * The default /api/v1/host answer stays helper-missing, because that notice is
 * the widest the route gets and the README picture shows it. With it, though,
 * the four buttons are disabled and the dialog never opens, so the phone user
 * who HAS the helper is the one nobody measured. The variant is a file of its
 * own, web/fixtures/host-helper-installed.json, and src/fixtures.test.ts parses
 * the same merge with hostSchema: a variant written only into this script
 * would be the one fixture no schema checks (ledger 149).
 */
const HOST_HELPER_INSTALLED = {
  ...FIXTURES['/api/v1/host'],
  actions: JSON.parse(
    readFileSync(
      fileURLToPath(new URL('../fixtures/host-helper-installed.json', import.meta.url)),
      'utf8',
    ),
  ),
}

const WIDTHS = [390, 1280]

/**
 * The width a thumb is measured at, and the size it needs.
 *
 * Only the phone: the desk keeps the compact sizes, which is the operator's
 * decision and the reason the sizes are split by breakpoint at all.
 */
const TOUCH_WIDTH = 390
const TOUCH_MIN = 44
/** The width the desk is dumped at (D-08): the other side of `md`. */
const DESK_WIDTH = 1280

/**
 * What counts as a control: one list, handed to findSmallTargets as an argument
 * for both the size pass and the D-08 dump, so the two cannot disagree about
 * what a control is. A second copy would drift, and a dump that counted other
 * elements than the audit measures would compare something nobody checks.
 */
const CONTROL_SELECTOR = [
  'button',
  'a[href]',
  'summary',
  'input',
  'select',
  'textarea',
  '[role="button"]',
  '[role="checkbox"]',
  '[role="switch"]',
  '[role="tab"]',
  '[role="menuitem"]',
  '[role="option"]',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

/**
 * Where the D-08 dump goes, when asked for; undefined otherwise.
 *
 * Resolved to an absolute path (a relative one is read from the directory the
 * script runs in, web/ under `task test:layout`) and refused inside the
 * repository before the daemon starts: D-08 forbids a checked-in baseline, and
 * the one way a dump becomes one by accident is being written where `git add`
 * finds it. The directory is resolved through its symlinks, so a link into the
 * checkout is refused too. Truncated here, so a dump holds one run.
 */
const DUMP = (() => {
  const asked = process.env.LAYOUT_DUMP
  if (!asked) return undefined
  const repo = realpathSync(fileURLToPath(new URL('../../', import.meta.url)))
  const wanted = resolve(asked)
  let folder
  try {
    folder = realpathSync(dirname(wanted))
  } catch {
    throw new Error(`LAYOUT_DUMP: the directory of ${wanted} does not exist`)
  }
  const file = join(folder, basename(wanted))
  const inside = relative(repo, file)
  if (inside === '' || (!inside.startsWith('..') && !isAbsolute(inside))) {
    throw new Error(
      `LAYOUT_DUMP: ${file} is inside the repository. A dump is a one-off measurement and ` +
        'never a checked-in baseline (D-08); write it outside, e.g. under ~/.cache.',
    )
  }
  writeFileSync(file, '')
  return file
})()
/**
 * The routes it measures, and where the list comes from.
 *
 * web/scripts/layout-routes.json, one entry per leaf route of the router, and
 * src/layoutRoutes.test.ts holds that list equal to the router: it runs ahead of
 * this script in `npm run test:layout`, so a route added to the app without an
 * entry here fails the gate before a browser starts. The list used to be typed
 * into this file by hand, and that is the shape of ledger 153 (three screens the
 * audit had never opened) and 159 (a build it never looked at): a guard reports
 * on what it was told about, and the router grew beside it.
 *
 * `route` is the router's path, `path` what the browser opens -- a parameter
 * route opens an example from web/fixtures/demo.json. `when` marks the two
 * screens measured before a session (before-account: /setup, before-session:
 * /login); every entry without it is measured signed in.
 *
 * The wall is measured like every other screen, and it is the one that most
 * looks like it does not need to be: it is fixed to the viewport and never
 * scrolls, so anything that does not fit is GONE rather than one swipe away.
 * The Kubernetes screen is ten pages since 2026-09-20 plus the apps list and one
 * app, and each is its own entry: listing only '/kubernetes' would measure the
 * overview and call the rest checked.
 */
const ROUTE_ENTRIES = JSON.parse(
  readFileSync(fileURLToPath(new URL('./layout-routes.json', import.meta.url)), 'utf8'),
)
const WHENS = ['before-account', 'before-session']
for (const e of ROUTE_ENTRIES) {
  if (e.when !== undefined && !WHENS.includes(e.when)) {
    throw new Error(
      `layout-routes.json: ${e.route} has when "${e.when}", which this audit does not read`,
    )
  }
}
const entryFor = (when) => {
  const e = ROUTE_ENTRIES.find((x) => x.when === when)
  if (e === undefined) throw new Error(`layout-routes.json has no entry with when "${when}"`)
  return e
}
const BEFORE_ACCOUNT = entryFor('before-account')
const BEFORE_SESSION = entryFor('before-session')
const SIGNED_IN = ROUTE_ENTRIES.filter((e) => e.when === undefined)

/**
 * What appears after a tap: the states a route pass never sees.
 *
 * An explicit list, not "click every button": clicking everything would press
 * the destructive actions this product is full of, and what opens would depend
 * on the order things were clicked in. Each opener names its route, the widths
 * it is measured at, the steps that open it (each with the name a missing
 * control is reported by), the selector of what opened, and optionally:
 * - `close`: `{ name, run(page) }`, Escape when absent;
 * - `fixture`: a per-opener override of GET answers, keyed by API path like
 *   web/fixtures/demo.json. Each path gets a page route of its own, installed
 *   before the page loads (a page route beats the context's) and removed after
 *   the close; non-GET requests pass through it untouched;
 * - `check(page, opened, w, label)`: a check of its own after the measurement,
 *   returning how many ways it failed;
 * - `enabled`: for a state with no open steps -- one the route already shows
 *   under its override -- the sentence printed when not one control inside
 *   `expect` is enabled, which means the override never reached the page. Such
 *   a state has nothing to close, so its close check is skipped.
 *
 * Opened states are measured scoped to what opened, after its animation has
 * finished, and must close again. The runner is further down (runOpener).
 *
 * NONE OF THEM CONFIRMS ANYTHING. The Power confirmation, the Reset dialog and
 * the host action dialog are opened, measured and closed: nothing is typed into
 * their confirmation fields and no confirm button in them is addressed. A Power
 * confirmation would reach the handler, a Reset would be a Reset, and the host
 * action chain would place an order. The request monitor is the proof.
 */
const OPENERS = [
  /**
   * The sudo dialog, reached the real way: the New account form on /settings.
   * POST /api/v1/users is admin-only, Destructive and has no cluster scope, so
   * the audit's own daemon answers `428 sudo.required` in its sudo middleware
   * before the user.create handler runs -- and a login never opens the sudo
   * window, only POST /api/v1/auth/sudo does.
   *
   * How "the audit never presses a confirm button" is read here: submitting a
   * form that the server answers 428 before its handler is how the prompt is
   * reached at all. The audit never presses the confirm button inside a
   * confirmation dialog, and never the sudo dialog's -- it never fills,
   * types into or submits the dialog's password field, because it knows the
   * account's password and submitting it would open the sudo window for the
   * rest of this context. The request monitor is the proof that nothing ran:
   * a 2xx to POST /api/v1/users would be an EXECUTED line and a red run, and
   * so would a 2xx to POST /api/v1/auth/sudo, the grant itself.
   *
   * The password typed into the form is a throwaway and not the audit
   * account's; the account it names is never created.
   */
  {
    name: 'Sudo dialog',
    route: '/settings',
    widths: [390, 1280],
    open: [
      {
        name: '#new-username',
        locate: (p) => p.locator('#new-username'),
        fill: 'layout-audit-probe',
      },
      {
        name: 'the New account password field',
        // /settings carries a second #new-password (the change-password form),
        // so the field is the one in the form that holds #new-username.
        locate: (p) => p.locator('form:has(#new-username) #new-password'),
        fill: 'a-throwaway-passphrase-never-created-2',
      },
      {
        name: 'Create account',
        locate: (p) => p.getByRole('button', { name: 'Create account', exact: true }),
      },
    ],
    expect: '[role="dialog"]:has-text("Confirm your password")',
  },
  /**
   * The phone's navigation drawer. Not a Radix dialog: nothing closes it on
   * Escape, and the backdrop's centre is covered by the drawer, so it is closed
   * the way a thumb closes it -- a tap on the backdrop beside the drawer. No
   * Escape handler is added to the drawer for the audit's sake: that would be a
   * behaviour change, and this phase changes tap targets only.
   *
   * Its own check: slid fully in, and every entry reachable. Thirteen areas at
   * 44px plus the brand, What's new and the notices come to about the height of
   * a 390x844 phone, and a drawer taller than the screen that does not scroll
   * has its last entries out of reach -- CUT OFF, not a skip.
   */
  {
    name: 'Navigation',
    route: '/',
    widths: [390],
    open: [
      {
        name: 'Open the navigation',
        locate: (p) => p.getByRole('button', { name: 'Open the navigation', exact: true }),
      },
    ],
    expect: 'nav[aria-label="Main navigation"]',
    check: checkDrawer,
    close: {
      name: 'a tap on "Close the navigation" beside the drawer',
      run: (p) =>
        p
          .getByRole('button', { name: 'Close the navigation', exact: true })
          .click({ position: { x: 370, y: 400 }, timeout: 5_000 }),
    },
  },
  /**
   * The release notes. On a phone the version button lives in the drawer, so
   * the drawer is opened first; on a desk it is in the permanent sidebar.
   */
  {
    name: "What's new",
    route: '/',
    widths: [390, 1280],
    open: (width) => [
      ...(width === TOUCH_WIDTH
        ? [
            {
              name: 'Open the navigation',
              locate: (p) => p.getByRole('button', { name: 'Open the navigation', exact: true }),
            },
          ]
        : []),
      {
        name: 'the version button titled "What changed in this release"',
        locate: (p) => p.getByTitle('What changed in this release', { exact: true }),
      },
    ],
    expect: `[role="dialog"]:has-text("What's New")`,
  },
  /** The node's Power menu, its unavailable items included (measured disabled). */
  {
    name: 'Power menu',
    route: '/nodes/m-cp-1',
    widths: [390, 1280],
    open: [{ name: 'Power: …', locate: powerTrigger }],
    expect: '[role="menu"]',
  },
  /**
   * The confirmation a Power item opens, from its first available item. Opened,
   * measured and closed: its confirm button is never pressed, because on this
   * route a non-forced action is not behind sudo and would reach the handler.
   */
  {
    name: 'Power confirmation',
    route: '/nodes/m-cp-1',
    widths: [390, 1280],
    open: [
      { name: 'Power: …', locate: powerTrigger },
      {
        name: 'the first available Power item',
        locate: (p) => p.locator('[role="menu"] [role="menuitem"]:not([data-disabled])').first(),
      },
    ],
    expect: '[role="dialog"]',
  },
  /**
   * The Reset dialog with its disk rows: the default mode needs disks, and
   * demo.json answers m-cp-1's reset preview as the server builds it. Nothing
   * is typed into #reset-confirm and its confirm button is never pressed.
   */
  {
    name: 'Reset dialog',
    route: '/nodes/m-cp-1',
    widths: [390, 1280],
    open: [
      {
        name: 'Reset',
        locate: (p) => p.getByRole('button', { name: 'Reset', exact: true }),
      },
    ],
    expect: '[role="dialog"]:has(input[type="checkbox"])',
  },
  /** A Radix Select list: its options are `role="option"` rows. */
  {
    name: 'Select list',
    route: '/audit',
    widths: [390, 1280],
    open: [{ name: '#audit-action', locate: (p) => p.locator('#audit-action') }],
    expect: '[role="listbox"]',
  },
  /**
   * /host's action group as a phone user with the helper meets it: every button
   * enabled. Nothing to open -- the route already shows it under the override --
   * and every button in the group is measured whatever their number, so a fifth
   * host action is measured without a change here.
   */
  {
    name: 'Host actions (helper installed)',
    route: '/host',
    widths: [390, 1280],
    fixture: { '/api/v1/host': HOST_HELPER_INSTALLED },
    open: [],
    expect: 'fieldset[aria-label="Host actions"]',
    enabled: 'the helper-installed answer did not reach the page',
  },
  /**
   * The host action dialog with its typed-hostname field. Nothing is typed into
   * #host-action-confirm, so its confirm button stays disabled and is measured
   * disabled; no confirm and no order is ever sent. A disabled "Restart host"
   * means the override did not apply, and the 5 s click timeout says so.
   */
  {
    name: 'Host action dialog',
    route: '/host',
    widths: [390, 1280],
    fixture: { '/api/v1/host': HOST_HELPER_INSTALLED },
    open: [
      {
        name: 'Restart host',
        locate: (p) => p.getByRole('button', { name: 'Restart host', exact: true }),
      },
    ],
    expect: '[role="dialog"]:has(#host-action-confirm)',
  },
]

/** The node's Power trigger: its accessible name is "Power: <node name>". */
function powerTrigger(p) {
  return p.getByRole('button', { name: /^Power: / })
}

/**
 * The drawer's own check, after it has finished sliding: fully in, and not
 * taller than the screen without scrolling.
 */
async function checkDrawer(_page, opened, w, label) {
  const m = await opened.evaluate((nav) => ({
    left: nav.getBoundingClientRect().left,
    scrollHeight: nav.scrollHeight,
    clientHeight: nav.clientHeight,
    overflowY: getComputedStyle(nav).overflowY,
  }))
  let found = 0
  if (Math.abs(m.left) > 0.5) {
    found += 1
    console.error(`  OPENER    ${w}  ${label} -- the drawer did not slide in`)
  }
  if (m.scrollHeight > m.clientHeight + 1 && m.overflowY !== 'auto' && m.overflowY !== 'scroll') {
    found += 1
    console.error(
      `  CUT OFF   ${w}  ${label} -- the drawer is ${m.scrollHeight}px tall in ${m.clientHeight}px and does not scroll`,
    )
  }
  return found
}

/**
 * The daemon this measures, and why it is built here rather than found here.
 *
 * It used to be `../bin/holzkube-managerd`, whatever that happened to be. On
 * 2026-09-20 that turned out to be seventeen hours old: three panels had been
 * added to /kubernetes, the audit reported the same 25 controls and 12 items as
 * before them, and exit 0. Nothing was wrong with the page. The guard was
 * photographing yesterday.
 *
 * That is the worst shape a guard can have -- it passes, and its passing is
 * about a build nobody is shipping -- and it is the same genus as the empty
 * fixtures (ledger 140) and the three missing screens (ledger 153): the
 * instrument, not the code.
 *
 * So this builds it. The binary EMBEDS web/dist, so the build is the only thing
 * that ties this measurement to the sources on disk, and `go build` on an
 * unchanged tree is a cache hit rather than a cost. `HOLZKUBE_BINARY` still
 * overrides, for a caller who has one and means it -- CI passes the artifact it
 * just built -- and the override is a deliberate act rather than a default.
 */
const BINARY = process.env.HOLZKUBE_BINARY ?? '../bin/holzkube-managerd'
const BUILD_IT = process.env.HOLZKUBE_BINARY === undefined

/**
 * Where the Chromium is.
 *
 * Left to Playwright wherever it installed its own -- which is what CI does,
 * with the copy web/package-lock.json pins. Only overridden when a browser is
 * provided at a fixed path, as in a container that ships one; hard-coding that
 * path would have made this script pass in one environment and fail to launch
 * in the other, which is a worse failure than a layout defect because it looks
 * like the guard itself is broken.
 */
const BROWSER =
  process.env.PLAYWRIGHT_CHROMIUM ??
  (existsSync('/opt/pw-browsers/chromium') ? '/opt/pw-browsers/chromium' : undefined)
const USER = 'layout-audit'
const PASSWORD = 'a-long-enough-passphrase-for-the-audit-1'

/** A port the kernel picked, so two runs on one machine cannot collide. */
async function freePort() {
  const net = await import('node:net')
  return new Promise((resolve, reject) => {
    const s = net.createServer()
    s.on('error', reject)
    s.listen(0, '127.0.0.1', () => {
      const { port } = s.address()
      s.close(() => resolve(port))
    })
  })
}

async function waitForDaemon(base, deadlineMs = 30_000) {
  const until = Date.now() + deadlineMs
  while (Date.now() < until) {
    try {
      const r = await fetch(`${base}/api/v1/system/status`)
      if (r.ok) return await r.json()
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 250))
  }
  throw new Error(`the daemon did not answer on ${base} within ${deadlineMs}ms`)
}

/**
 * Finds every element the viewport cuts off with no way to scroll it back.
 *
 * Run inside the page, so it sees computed styles and real rectangles rather
 * than class names -- the distinction that made the difference here, because
 * the page never scrolled sideways and nothing about the markup said so.
 */
const findClipped = (root, vw) => {
  const out = []
  for (const el of root.querySelectorAll('*')) {
    const box = el.getBoundingClientRect()
    if (box.width < 1 && box.height < 1) continue
    if (box.right <= vw + 1) continue
    const style = getComputedStyle(el)
    if (style.visibility === 'hidden' || style.display === 'none') continue

    let ancestor = el.parentElement
    let reachable = false
    while (ancestor) {
      const overflow = getComputedStyle(ancestor).overflowX
      if (overflow === 'auto' || overflow === 'scroll') {
        reachable = ancestor.scrollWidth > ancestor.clientWidth + 1
        break
      }
      ancestor = ancestor.parentElement
    }
    if (reachable) continue

    out.push({
      tag: el.tagName.toLowerCase(),
      right: Math.round(box.right),
      cls: (el.getAttribute('class') ?? '').slice(0, 60),
      text: (el.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 40),
    })
  }
  return out
}

/**
 * Finds a fixed pane that scrolls SIDEWAYS.
 *
 * findClipped forgives anything inside a sideways-scrolling ancestor, on the
 * reasoning that a table one can swipe is reachable. The wall's root IS such an
 * ancestor -- it scrolls below `md`, because on a phone "never scroll" would
 * mean "silently cut off". So on a phone the wall ran a third of its width off
 * the right edge, the node names and the warnings with it, and this guard
 * reported "nothing out of reach" for it five runs in a row.
 *
 * Vertical scrolling on a phone is how a phone works. Sideways scrolling is the
 * thing this guard already refuses for tables, and a full-screen pane doing it
 * is worse: there is no edge to tell somebody there is more.
 */
const findSidewaysPanes = (root) => {
  const out = []
  // The root itself too: an opened dialog IS a fixed pane, and scoping the pass
  // to it must not skip the one element it is about.
  for (const el of [root, ...root.querySelectorAll('*')]) {
    const style = getComputedStyle(el)
    if (style.position !== 'fixed' && style.position !== 'absolute') continue
    if (el.scrollWidth <= el.clientWidth + 1) continue
    // A pane, not a sliver: `sr-only` and the hidden labels Radix ships are a
    // deliberate 1px with their text clipped, and reporting those would drown
    // the finding that matters in artefacts of the accessibility layer.
    if (el.clientWidth < 100) continue

    // WHICH descendant sticks out, not only that something does. A guard that
    // says "this pane scrolls sideways" sends somebody hunting; the widest
    // thing past the edge is usually the answer itself.
    const edge = el.getBoundingClientRect().right
    const culprits = []
    for (const kid of el.querySelectorAll('*')) {
      const box = kid.getBoundingClientRect()
      if (box.width < 2 || box.right <= edge + 1) continue
      if (kid.querySelector('*') !== null) continue // the innermost one only
      culprits.push({
        tag: kid.tagName.toLowerCase(),
        right: Math.round(box.right),
        cls: (kid.getAttribute('class') ?? '').slice(0, 70),
        text: (kid.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 40),
      })
    }
    out.push({
      tag: el.tagName.toLowerCase(),
      scroll: el.scrollWidth,
      client: el.clientWidth,
      cls: (el.getAttribute('class') ?? '').slice(0, 70),
      culprits: culprits.slice(0, 4),
    })
  }
  return out
}

/**
 * Finds every control a thumb cannot hit.
 *
 * The operator's decision of 2026-09-17: 44px below `md`, the desk left alone.
 * WCAG 2.5.8 asks for 24; 44 is what Apple and Material name for fingers, and
 * a cluster page is read one-handed while something is broken.
 *
 * DISABLED CONTROLS ARE MEASURED. A disabled control becomes enabled without a
 * layout change -- the four /host buttons the moment the helper is installed, a
 * confirm button the moment the hostname is typed -- and the size it has then
 * is the size it has now. Skipping them measured the screen in the one state
 * nobody acts in and left the buttons that matter unmeasured.
 *
 * What is skipped, all of it, each for a reason that is about the element not
 * being a target at all (D-09) -- not a softening of the size rule:
 * - `visibility: hidden` and `display: none`: not on the screen, nothing to hit
 *   (the shut drawer is `visibility: hidden`).
 * - `aria-hidden="true"` on the element itself: a decorative copy the product
 *   has taken out of the accessibility tree on purpose; only the element's own
 *   attribute counts, an ancestor's does not.
 * - a zero-sized box: the hidden native <select> a Radix trigger keeps for form
 *   semantics. The visible target is the trigger beside it, measured on its own.
 * - a link inside a sentence, which WCAG 2.5.8 exempts by name because growing
 *   it to 44px wrecks the line it sits in. It still counts as measured.
 * Anything else counts. A new exemption goes on this list with its reason.
 *
 * Each finding carries the element's `data-slot` and `data-size` when it has
 * them, so a line says which primitive to raise rather than which screen.
 */
const findSmallTargets = (root, { min, selector, dump = false }) => {
  const out = []
  // With `dump`, every control that passes the skips, in DOM order, with the
  // element's OWN box: wrapping a checkbox in a label must not read as a moved
  // control in the D-08 comparison, even though the label is what is tapped.
  const controls = []
  // How many controls passed the visibility and size skips, from the same loop and
  // the same selector, so a menu of `role="menuitem"` rows or a list of options counts
  // what it holds. A narrower count selector would call a full menu empty.
  let measured = 0
  for (const el of root.querySelectorAll(selector)) {
    const style = getComputedStyle(el)
    if (style.visibility === 'hidden' || style.display === 'none') continue
    if (el.getAttribute('aria-hidden') === 'true') continue

    // The box that is actually tapped. A checkbox inside a <label> is not the
    // target: clicking anywhere on the label toggles it, so the label is what
    // a thumb aims at and what has to be big enough.
    const wrapper = el.closest('label')
    const target = wrapper !== null && wrapper !== el ? wrapper : el
    const box = target.getBoundingClientRect()
    // Zero-sized: the hidden native select a Radix trigger keeps for form
    // semantics, and anything else with no box. Its visible partner is the
    // button beside it, which this pass measures on its own.
    if (box.width < 1 || box.height < 1) continue
    // Counted here: it passed the visibility and size skips, so it is a control
    // on the screen. The exemption for a link in running text below is a
    // verdict about its size, not a sign that nothing is there.
    measured += 1
    if (dump) {
      const own = el.getBoundingClientRect()
      controls.push({
        tag: el.tagName.toLowerCase(),
        name: (el.getAttribute('aria-label') ?? el.textContent ?? '')
          .trim()
          .replace(/\s+/g, ' ')
          .slice(0, 40),
        x: Math.round(own.x),
        y: Math.round(own.y),
        width: Math.round(own.width),
        height: Math.round(own.height),
      })
    }
    // A link in running text. The test is the sentence around it: a parent
    // that carries more text than the link does is the sentence.
    if (el.tagName === 'A' && style.display.startsWith('inline')) {
      const own = (el.textContent ?? '').trim()
      const parent = (el.parentElement?.textContent ?? '').trim()
      if (own.length > 0 && parent.length > own.length) continue
    }
    if (box.width >= min && box.height >= min) continue

    out.push({
      tag: el.tagName.toLowerCase(),
      w: Math.round(box.width),
      h: Math.round(box.height),
      via: target === el ? '' : ' (via its label)',
      label: (el.getAttribute('aria-label') ?? el.textContent ?? el.getAttribute('name') ?? '')
        .trim()
        .replace(/\s+/g, ' ')
        .slice(0, 34),
      cls: (el.getAttribute('class') ?? '').slice(0, 50),
      // The primitive it came from: the element's own slot, or its label's
      // when the label is the target and the element names none.
      primitive: [
        el.getAttribute('data-slot') ?? target.getAttribute('data-slot'),
        el.getAttribute('data-size') ?? target.getAttribute('data-size'),
      ]
        .filter((x) => x !== null && x !== '')
        .join(' '),
    })
  }
  return { small: out, measured, controls }
}

/**
 * Finds every table a phone cannot read without losing the row.
 *
 * This is the hole the guard had, and it was an explicit decision rather than an
 * oversight: findClipped forgives anything inside a sideways-scrolling ancestor,
 * on the argument that a wide table is "ugly and not broken". A photograph from
 * the operator's phone on 2026-09-19 settled that argument the other way. The
 * screen showed a "Scale" field and a "Roll pods" button with the deployment's
 * namespace and name scrolled off the left edge: the buttons were reachable and
 * it was impossible to say WHICH deployment they would act on.
 *
 * That is not ugly, it is dangerous, and it is the reason this pass exists. A
 * table whose content is wider than the viewport at phone width is a finding.
 *
 * Only at phone width: a wide table on a desk is what tables are for.
 */
const findWideTables = (root, vw) => {
  const out = []
  for (const el of root.querySelectorAll('table')) {
    const style = getComputedStyle(el)
    if (style.visibility === 'hidden' || style.display === 'none') continue
    const box = el.getBoundingClientRect()
    if (box.width < 1 && box.height < 1) continue

    // Its own content wider than the screen, whether it is the table that
    // scrolls or the wrapper around it.
    const wrapper = el.parentElement
    const scrolls =
      el.scrollWidth > el.clientWidth + 1 ||
      (wrapper !== null && wrapper.scrollWidth > wrapper.clientWidth + 1)
    if (!scrolls && box.width <= vw + 1) continue

    const heading = el.closest('[data-slot="card"]')?.querySelector('[data-slot="card-title"]')
    out.push({
      width: Math.round(Math.max(el.scrollWidth, box.width)),
      rows: el.querySelectorAll('tbody tr').length,
      where: (heading?.textContent ?? el.querySelector('caption')?.textContent ?? '')
        .trim()
        .slice(0, 40),
      first: (el.querySelector('tbody tr')?.textContent ?? '')
        .trim()
        .replace(/\s+/g, ' ')
        .slice(0, 40),
    })
  }
  return out
}

/**
 * How much the page actually rendered, so an empty screen cannot pass as a
 * clean one.
 *
 * Table rows, phone rows and cards all count: several screens (jobs, clusters)
 * have never been tables, and reporting "0 rows" for a screen full of cards
 * would be the same ambiguity the row count exists to remove -- rendered
 * nothing, or rendered something this selector cannot see?
 */
const countItems = () =>
  document.querySelectorAll('tbody tr, [data-row], [data-slot="card"]').length

/**
 * A picture of the phone, when asked for. Never part of a verdict: the guard
 * measures, and a person looking at a screenshot is a weaker check. It exists
 * because the operator reported this in a photograph, and a photograph is what
 * answers one.
 *
 * The viewport is grown to the content rather than `fullPage`, and that was
 * measured the hard way: this shell scrolls inside `<main class="flex-1
 * min-h-0 overflow-auto">`, not the document. `fullPage` expands the DOCUMENT,
 * the inner container keeps its height, and the result is a tall photograph of
 * an empty page -- a picture of /settings that looked like the accounts were
 * missing when they were one swipe away. Setting a height on that element does
 * not work either: it is a flex child and the layout overrides it. Growing the
 * window is the one that works, because the shell is built to fill it.
 *
 * It runs after both measuring passes, so resizing cannot change a verdict.
 */
async function shoot(page, route, width) {
  if (!process.env.LAYOUT_SHOTS || width !== TOUCH_WIDTH) return

  // The wall is the one screen whose target is a television, so a phone-width
  // photograph of it shows nobody anything. It gets a shot at the size it is
  // actually read at. The MEASURING passes are unchanged and still run at both
  // ordinary widths -- this is a picture, and a picture is never a verdict.
  if (route === '/wall') {
    await page.setViewportSize({ width: 1920, height: 1080 })
    await page.waitForTimeout(400)
    await page.screenshot({ path: `${process.env.LAYOUT_SHOTS}/wall.png` })
    // And one at phone width, because that is where the operator looked at it
    // first and where a layout built for a television has to degrade rather
    // than fall apart.
    await page.setViewportSize({ width, height: 844 })
    await page.waitForTimeout(400)
    await page.screenshot({ path: `${process.env.LAYOUT_SHOTS}/wall-phone.png`, fullPage: true })
    await page.waitForTimeout(150)
    return
  }

  const content = await page.evaluate(() => {
    let tallest = document.documentElement.scrollHeight
    for (const el of document.querySelectorAll('*')) {
      const style = getComputedStyle(el)
      if (style.overflowY === 'auto' || style.overflowY === 'scroll') {
        tallest = Math.max(tallest, el.scrollHeight + el.getBoundingClientRect().top)
      }
    }
    return Math.ceil(tallest)
  })

  const height = Math.min(Math.max(content, 844), 6000)
  await page.setViewportSize({ width, height })
  await page.waitForTimeout(250)
  const name = route === '/' ? 'home' : route.replace(/\//g, '')
  await page.screenshot({ path: `${process.env.LAYOUT_SHOTS}/${name}.png` })
  await page.setViewportSize({ width, height: 844 })
  await page.waitForTimeout(150)
}

if (BUILD_IT) {
  // The BUNDLE first, and for the same reason as the binary below.
  //
  // The daemon serves web assets that were embedded at compile time from
  // internal/httpapi/dist, and `go build` does not put them there -- vite does.
  // So building only the binary measures whatever bundle somebody happened to
  // build last, and on 2026-09-20 that is exactly what happened: this audit
  // photographed a wall that had been replaced an hour earlier, and exited 0.
  // It is the third time this instrument has reported on something other than
  // the code in front of it (ledger 159, 170, and this).
  const bundled = spawnSync('npm', ['run', 'build'], { cwd: '.', stdio: 'inherit' })
  if (bundled.status !== 0) {
    throw new Error(
      `could not build the web bundle (npm run build exited ${bundled.status ?? 'without running'}). ` +
        'This audit measures the bundle it builds, because a stale one photographs the page as it ' +
        'was yesterday and exits 0.',
    )
  }

  // Built from here, so what is measured is what is on disk. A failure is fatal
  // rather than a fall back to whatever was there: measuring the old one is
  // exactly the outcome this exists to prevent.
  const built = spawnSync(
    'go',
    ['build', '-o', 'bin/holzkube-managerd', './cmd/holzkube-managerd'],
    {
      cwd: '..',
      stdio: 'inherit',
    },
  )
  if (built.status !== 0) {
    throw new Error(
      `could not build the daemon (go build exited ${built.status ?? 'without running'}). ` +
        'This audit measures the binary it builds, because a stale one reports the page as it was ' +
        'yesterday and exits 0.',
    )
  }
}

const dir = await mkdtemp(join(tmpdir(), 'holzkube-layout-'))
const port = await freePort()
const base = `http://127.0.0.1:${port}`

const daemon = spawn(
  BINARY,
  ['--insecure-http', '--listen', `127.0.0.1:${port}`, '--data-dir', dir, '--log-level', 'error'],
  { stdio: ['ignore', 'pipe', 'pipe'] },
)
let daemonOutput = ''
daemon.stdout.on('data', (d) => {
  daemonOutput += d
})
daemon.stderr.on('data', (d) => {
  daemonOutput += d
})
daemon.on('exit', (code) => {
  if (code !== null && code !== 0) {
    console.error(`the daemon exited with ${code}:\n${daemonOutput}`)
  }
})

let browser
let failures = 0
// What was actually measured, so the green line counts routes opened rather
// than routes listed.
const measuredRoutes = new Set()
const measuredOpeners = new Set()
// How many D-08 lines were written, printed at the end when LAYOUT_DUMP is set.
let dumped = 0
try {
  await waitForDaemon(base)

  browser = await chromium.launch(BROWSER ? { executablePath: BROWSER } : {})

  /**
   * One context, with the data already stubbed.
   *
   * GET only: setup and login go to the real daemon, so the session and the
   * shell stay the product's own.
   */
  async function open(width) {
    const context = await browser.newContext({ viewport: { width, height: 844 } })
    await context.route('**/api/v1/**', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      const path = new URL(route.request().url()).pathname
      let body = FIXTURES[path]
      if (body === undefined) return route.continue()

      // The wall dims itself when its answer is older than a few refreshes,
      // which is the whole of its honesty -- and a fixture carries a fixed
      // timestamp, so every run photographed a permanently stale screen. The
      // staleness is still measured by the component's own tests; what this
      // gives back is a picture of the ordinary state, which is the one the
      // layout has to be judged in.
      if (path.endsWith('/wall')) {
        body = { ...body, generated_at: new Date().toISOString() }
      }
      return route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        body: JSON.stringify(body),
      })
    })
    return context
  }

  /**
   * Runs the passes inside one scope: the page's <body> for a route, the element
   * that opened for an opener. A real scope, handed to the in-page functions as
   * their root -- not a document-wide pass filtered afterwards, because Radix
   * leaves the page in the DOM behind a modal and a document-wide pass would
   * measure the route a second time and report its findings twice.
   */
  async function scan(scope, width) {
    const result = { clipped: await scope.evaluate(findClipped, width) }
    if (width === TOUCH_WIDTH) {
      result.wide = await scope.evaluate(findWideTables, width)
      result.sideways = await scope.evaluate(findSidewaysPanes)
      const touch = await scope.evaluate(findSmallTargets, {
        min: TOUCH_MIN,
        selector: CONTROL_SELECTOR,
      })
      result.small = touch.small
      result.measured = touch.measured
    }
    return result
  }

  /**
   * Appends the D-08 lines for one scope at the desk width: the page's <body>
   * for a route (opener empty), the element that opened for an opener. The
   * index is the control's place in DOM order within that scope -- the one
   * order that is the same on two runs of the same tree. Nothing without
   * LAYOUT_DUMP, and nothing at the phone width.
   */
  async function dumpControls(scope, route, opener, width) {
    if (DUMP === undefined || width !== DESK_WIDTH) return
    const { controls } = await scope.evaluate(findSmallTargets, {
      min: TOUCH_MIN,
      selector: CONTROL_SELECTOR,
      dump: true,
    })
    const lines = controls.map((c, index) => JSON.stringify({ route, opener, index, ...c }))
    if (lines.length > 0) appendFileSync(DUMP, `${lines.join('\n')}\n`)
    dumped += lines.length
  }

  /** Prints what a scan found under `label`, and returns how many ways it failed. */
  function report(label, width, result) {
    let found = 0
    const w = `${String(width).padStart(4)}px`
    const { clipped, wide = [], sideways = [], small = [] } = result
    if (wide.length > 0) {
      found += 1
      console.error(`  SIDEWAYS  ${w}  ${label}  (${wide.length})`)
      for (const t of wide) {
        console.error(
          `              ${t.width}px wide, ${t.rows} rows, in "${t.where}" -- first row: "${t.first}"`,
        )
      }
    }
    if (sideways.length > 0) {
      found += 1
      console.error(`  OFFSCREEN ${w}  ${label}  (${sideways.length})`)
      for (const p of sideways) {
        console.error(
          `              <${p.tag}> ${p.scroll}px of content in ${p.client}px .${p.cls}`,
        )
        for (const c of p.culprits) {
          console.error(
            `                past the edge: <${c.tag}> right=${c.right} "${c.text}" .${c.cls}`,
          )
        }
      }
    }
    if (small.length > 0) {
      found += 1
      console.error(`  SMALL     ${w}  ${label}  (${small.length})`)
      for (const c of small) {
        const primitive = c.primitive === '' ? '' : ` [${c.primitive}]`
        console.error(
          `              <${c.tag}>${c.via} ${c.w}x${c.h}${primitive} "${c.label}" .${c.cls}`,
        )
      }
    }
    if (clipped.length > 0) {
      found += 1
      console.error(`  CLIPPED   ${w}  ${label}  (${clipped.length})`)
      for (const c of clipped.slice(0, 5)) {
        console.error(`              <${c.tag}> right=${c.right} "${c.text}" .${c.cls}`)
      }
    }
    return found
  }

  /**
   * A page as measure() leaves it: loaded, its queries landed, settled.
   *
   * The shell renders, then the queries land and the page grows. Measuring
   * before that is measuring an empty screen, which passes everything -- and a
   * fixed wait is the flake one builds oneself: 700ms was enough on a CI runner
   * and not on the operator's Pi, where /images measured 15 controls instead of
   * 32 because the Image Factory catalog had not arrived. Network idle first,
   * then a short settle for the render the last response triggers.
   */
  async function settle(page, route) {
    await page.goto(base + route)
    await page.waitForLoadState('networkidle', { timeout: 20_000 }).catch(() => {})
    await page.waitForTimeout(400)
  }

  /** Measures one route at one width, and returns how many ways it failed. */
  async function measure(page, route, width) {
    measuredRoutes.add(route)
    await settle(page, route)

    const result = await scan(page.locator('body'), width)
    await dumpControls(page.locator('body'), route, '', width)
    const counted =
      width === TOUCH_WIDTH
        ? `  (${result.measured} controls, ${await page.evaluate(countItems)} items)`
        : ''
    // After the passes and before the verdict, so resizing for a picture
    // cannot change what was measured, and so a route WITH findings is
    // photographed too -- that is the one somebody wants to look at.
    await shoot(page, route, width)

    const found = report(route, width, result)
    if (found === 0) {
      // The count is printed because this guard measures what the page
      // happens to show. /images lists one control per Image Factory
      // extension, and a catalog that did not load measures as a clean run:
      // that is how 17 undersized rows passed here and failed in CI.
      console.log(`  ok        ${String(width).padStart(4)}px  ${route}${counted}`)
    }
    return found
  }

  /**
   * Prints every non-GET API request the page answers while an opener runs,
   * and fails the run on one that carried an action out.
   *
   * Attached for the span of one opener only: setup and login are POSTs that
   * answer 2xx by design, and they happen outside any opener, so nothing an
   * opener sends needs /api/v1/auth/ or /api/v1/setup. The one path let
   * through is one ending in /confirm, which only issues a token that a
   * second, sudo-gated request would spend. A 2xx to POST /api/v1/auth/sudo
   * is EXECUTED like any other (14-REVIEW WR-02): it opens the sudo window for
   * the rest of the context, which is exactly what the Sudo dialog opener
   * promises never to do, and every later destructive opener would then reach
   * its handler.
   *
   * Why this exists at all: the audit runs on the machine production runs on.
   * It talks only to the daemon it started, in a temporary directory, and the
   * server's sudo gate stands between a submitted form and its handler. This
   * is the proof that the gate held, rather than a hope that it did.
   */
  function watchRequests(page) {
    let executed = 0
    const onResponse = (response) => {
      const request = response.request()
      const method = request.method()
      if (method === 'GET') return
      const path = new URL(response.url()).pathname
      if (!path.startsWith('/api/v1/')) return
      const status = response.status()
      console.log(`  REQUEST   ${method} ${path} -> ${status}`)
      const allowed = path.endsWith('/confirm')
      if (status >= 200 && status < 300 && !allowed) {
        executed += 1
        console.error(
          `  EXECUTED  ${method} ${path} -> ${status} -- the audit carried out an action; it must only open and measure`,
        )
      }
    }
    page.on('response', onResponse)
    return () => {
      page.off('response', onResponse)
      return executed
    }
  }

  /** Waits until every finite animation below `el` has finished (zoom-in-95 reads 44px as ~42). */
  const animationsDone = (el) =>
    Promise.all(
      el
        .getAnimations({ subtree: true })
        .filter((a) => a.effect?.getTiming().iterations !== Number.POSITIVE_INFINITY)
        .map((a) => a.finished.catch(() => {})),
    ).then(() => true)

  /**
   * Opens one state on a fresh page, measures it scoped, closes it, and
   * returns how many ways it failed.
   *
   * A fresh page per opener, loaded and settled as measure() does, so no state
   * leaks from the one before: an open drawer, a Problem left by a refused
   * request. Every way an opener can fail to be measured is a failure of its
   * own, never a skip (D-04).
   */
  async function runOpener(context, opener, width) {
    const label = `${opener.route} · ${opener.name}`
    const page = await context.newPage()
    // One page route per overridden path, installed before the page loads and
    // kept for the whole opener: /host polls every 3 s, and an override that
    // lapsed mid-opener would flip the page back to the default answer. A page
    // route beats the context's, so this replaces only what it names; any
    // method other than GET passes through to the context and the daemon.
    const overrides = Object.entries(opener.fixture ?? {}).map(([path, body]) => ({
      match: (url) => url.pathname === path,
      handler: (route) => {
        if (route.request().method() !== 'GET') return route.fallback()
        return route.fulfill({
          status: 200,
          contentType: 'application/json; charset=utf-8',
          body: JSON.stringify(body),
        })
      },
    }))
    for (const o of overrides) await page.route(o.match, o.handler)
    const stopWatching = watchRequests(page)
    let outcome
    let executed = 0
    try {
      outcome = await openMeasureClose(page, opener, width, label)
    } finally {
      executed = stopWatching()
      for (const o of overrides) await page.unroute(o.match, o.handler)
      await page.close()
    }
    const found = outcome.found + executed
    if (found === 0) {
      console.log(
        `  ok        ${String(width).padStart(4)}px  ${label}  (${outcome.measured} controls)`,
      )
    }
    return found
  }

  async function openMeasureClose(page, opener, width, label) {
    const w = `${String(width).padStart(4)}px`
    await settle(page, opener.route)
    const steps = typeof opener.open === 'function' ? opener.open(width) : opener.open
    for (const step of steps) {
      const target = step.locate(page)
      // Waited for rather than counted at once: a menu item exists only after
      // the click before it has rendered the menu, and a list of Power actions
      // only once the page has asked what is possible.
      try {
        await target.first().waitFor({ state: 'attached', timeout: 5_000 })
      } catch {
        console.error(`  OPENER    ${w}  ${label} -- no control named "${step.name}" to open it`)
        return { found: 1 }
      }
      try {
        if (step.fill !== undefined) await target.fill(step.fill, { timeout: 5_000 })
        else await target.click({ timeout: 5_000 })
      } catch (error) {
        const why = String(error?.message ?? error).split('\n')[0]
        console.error(`  OPENER    ${w}  ${label} -- could not use "${step.name}": ${why}`)
        return { found: 1 }
      }
    }

    const opened = page.locator(opener.expect)
    try {
      await opened.waitFor({ state: 'visible', timeout: 5_000 })
    } catch {
      console.error(
        `  OPENER    ${w}  ${label} -- clicked, but ${opener.expect} did not appear within 5 s`,
      )
      return { found: 1 }
    }
    // Measured once it has stopped moving: getBoundingClientRect includes the
    // transform, and a 44px button reads about 42px during zoom-in-95.
    await opened.evaluate(animationsDone)

    // A state with no open steps is what the route shows under the opener's
    // override. Not one enabled control in it means the override never reached
    // the page, and measuring the default answer under this name would call
    // the enabled buttons measured when nobody saw them.
    if (opener.enabled !== undefined) {
      const enabled = await opened.evaluate(
        (root) =>
          [...root.querySelectorAll('button, input, select, textarea, [role="button"]')].filter(
            (el) => !el.disabled && el.getAttribute('aria-disabled') !== 'true',
          ).length,
      )
      if (enabled === 0) {
        console.error(`  OPENER    ${w}  ${label} -- ${opener.enabled}`)
        return { found: 1 }
      }
    }
    measuredOpeners.add(opener.name)
    await dumpControls(opened, opener.route, opener.name, width)

    const result = await scan(opened, width)
    let found = report(label, width, result)
    if (opener.check !== undefined) found += await opener.check(page, opened, w, label)
    // Zero is red at both widths: a dialog with no control in it cannot be
    // closed by a thumb, and "measured nothing" printed as a count nobody reads
    // is the empty-output-is-green shape. The desk pass measures reach only, so
    // it counts with the same loop without reporting sizes.
    const measured =
      result.measured ??
      (await opened.evaluate(findSmallTargets, { min: TOUCH_MIN, selector: CONTROL_SELECTOR }))
        .measured
    if (measured === 0) {
      found += 1
      console.error(`  EMPTY     ${w}  ${label} -- opened, but not one control in it was measured`)
    }

    // Nothing was opened, so nothing is closed: the state is the route itself.
    if (steps.length === 0) return { found, measured }

    const close = opener.close ?? { name: 'Escape', run: (p) => p.keyboard.press('Escape') }
    try {
      await close.run(page)
      await opened.waitFor({ state: 'hidden', timeout: 5_000 })
    } catch {
      found += 1
      console.error(`  OPENER    ${w}  ${label} -- still open after ${close.name}`)
    }
    return { found, measured }
  }

  // THE TWO SCREENS BEFORE A SESSION, and they were missing for as long as this
  // guard has existed -- while being the first two anybody ever sees, usually on
  // a phone while standing somewhere. They cannot be one list with the rest,
  // because WHEN they are measured is what makes them measurable at all: /setup
  // exists only while no account does, so it goes first of all, against the
  // daemon as an operator meets it on the very first day.
  for (const width of WIDTHS) {
    const context = await open(width)
    const page = await context.newPage()
    failures += await measure(page, BEFORE_ACCOUNT.path, width)
    await context.close()
  }

  const setup = await fetch(`${base}/api/v1/setup`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Holzkube-Manager-CSRF': '1' },
    body: JSON.stringify({ username: USER, password: PASSWORD }),
  })
  if (!setup.ok) {
    throw new Error(`could not create the audit account: ${setup.status} ${await setup.text()}`)
  }

  // /login next, with an account but no session.
  for (const width of WIDTHS) {
    const context = await open(width)
    const page = await context.newPage()
    failures += await measure(page, BEFORE_SESSION.path, width)
    await context.close()
  }

  for (const width of WIDTHS) {
    const context = await open(width)
    const page = await context.newPage()

    await page.goto(`${base}/login`)
    await page.fill('#login-username', USER)
    await page.fill('#login-password', PASSWORD)
    await page.click('button[type="submit"]')
    await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 20_000 })

    for (const entry of SIGNED_IN) {
      failures += await measure(page, entry.path, width)
    }
    // After the routes, in the same signed-in context, each on a page of its own.
    for (const opener of OPENERS) {
      if (!opener.widths.includes(width)) continue
      failures += await runOpener(context, opener, width)
    }
    await context.close()
  }
} finally {
  await browser?.close()
  // Wait for the daemon to be gone before its directory is removed: since
  // Phase 12 it writes its metrics history into that directory on SIGTERM, and
  // a recursive rm racing that write fails with ENOTEMPTY -- a red audit after
  // every route passed. Bounded, so a daemon that hangs cannot hang the gate.
  const gone =
    daemon.exitCode !== null || daemon.signalCode !== null
      ? Promise.resolve()
      : new Promise((resolve) => daemon.once('exit', resolve))
  daemon.kill('SIGTERM')
  await Promise.race([gone, new Promise((resolve) => setTimeout(resolve, 10_000))])
  await rm(dir, { recursive: true, force: true })
}

if (DUMP !== undefined) {
  console.log(`\nLAYOUT_DUMP: ${dumped} controls at ${DESK_WIDTH}px written to ${DUMP}`)
}
if (failures > 0) {
  console.error(
    `\n${failures} finding(s) across routes, openers and widths.\n\n` +
      'An element past the right edge with nothing to scroll is not narrow, it is gone: ' +
      'no swipe brings it back and no scrollbar says it is there. That is how the ' +
      '"New account" form and the change-password button left /settings on a phone ' +
      'without anything appearing to be wrong.\n\n' +
      'Either let it wrap, or put it in a container that scrolls sideways.\n\n' +
      'A control under 44px at 390px is the other failure: WCAG 2.5.8 asks for 24, ' +
      'Apple and Material name 44 for a finger, and the operator chose 44 below md ' +
      'with the desk left alone. Raise it at the primitive rather than per screen. ' +
      'What is measured is the box a thumb actually aims at: a checkbox inside a ' +
      'label is measured as its label.\n\n' +
      'A SIDEWAYS table is the third: wider than the phone, so reading it means ' +
      'swiping the row identity off the left edge. The operator photographed ' +
      'exactly that on 2026-09-19 -- a Scale field and a Roll pods button with no ' +
      'way to see which deployment they belonged to. Below md a row becomes a ' +
      'card, or its secondary columns fold away; it does not become a swipe.\n\n' +
      'An opener that finds nothing is a failure, not a skip: a dialog the audit cannot open ' +
      'is a dialog it has not measured.',
  )
  process.exit(1)
}
console.log(
  `\nNothing out of reach at ${WIDTHS.join('px and ')}px, ` +
    `every control is at least ${TOUCH_MIN}px at ${TOUCH_WIDTH}px, ` +
    `on ${measuredRoutes.size} routes and in ${measuredOpeners.size} opened states.`,
)

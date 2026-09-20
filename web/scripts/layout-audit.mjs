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
 * WHY IT IS IN THE CHAIN AND NOT OPT-IN: ledger entries 5 and 64 record a drift
 * watcher that existed, was never scheduled, and therefore never watched
 * anything. A guard nobody runs is a comment.
 *
 * The widths are the operator's decision of 2026-09-17: a phone and a desk, the
 * two sides of the `md` breakpoint.
 */
import { spawn, spawnSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
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

const WIDTHS = [390, 1280]

/**
 * The width a thumb is measured at, and the size it needs.
 *
 * Only the phone: the desk keeps the compact sizes, which is the operator's
 * decision and the reason the sizes are split by breakpoint at all.
 */
const TOUCH_WIDTH = 390
const TOUCH_MIN = 44
const ROUTES = [
  '/', '/nodes', '/nodes/m-cp-1', '/clusters', '/kubernetes', '/config', '/jobs',
  '/provision', '/upgrades', '/images', '/audit', '/settings',
]


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
const findClipped = (vw) => {
  const out = []
  for (const el of document.querySelectorAll('body *')) {
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
 * Finds every control a thumb cannot hit.
 *
 * The operator's decision of 2026-09-17: 44px below `md`, the desk left alone.
 * WCAG 2.5.8 asks for 24; 44 is what Apple and Material name for fingers, and
 * a cluster page is read one-handed while something is broken.
 *
 * The exemptions are not softenings, they are the artefacts the first
 * measurement of this was mostly made of: a hidden native <select> behind a
 * Radix trigger (the visible target is the button, and it is measured), and a
 * link inside a sentence, which WCAG 2.5.8 exempts by name because growing it
 * to 44px wrecks the line it sits in. Anything else counts.
 */
const findSmallTargets = (min) => {
  const SELECTOR = [
    'button', 'a[href]', 'summary', 'input', 'select', 'textarea',
    '[role="button"]', '[role="checkbox"]', '[role="switch"]', '[role="tab"]',
    '[role="menuitem"]', '[role="option"]', '[tabindex]:not([tabindex="-1"])',
  ].join(',')

  const out = []
  for (const el of document.querySelectorAll(SELECTOR)) {
    const style = getComputedStyle(el)
    if (style.visibility === 'hidden' || style.display === 'none') continue
    if (el.getAttribute('aria-hidden') === 'true') continue
    if (el.hasAttribute('disabled')) continue

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
      label: ((el.getAttribute('aria-label') ?? el.textContent ?? el.getAttribute('name') ?? '')
        .trim()
        .replace(/\s+/g, ' ')
        .slice(0, 34)),
      cls: (el.getAttribute('class') ?? '').slice(0, 50),
    })
  }
  return out
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
const findWideTables = (vw) => {
  const out = []
  for (const el of document.querySelectorAll('table')) {
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
      first: (el.querySelector('tbody tr')?.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 40),
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

/** How many controls the touch pass looked at, so a thin page is visible. */
const countTargets = () =>
  [...document.querySelectorAll('button,a[href],summary,input,select,textarea,[role="button"]')].filter(
    (el) => {
      const style = getComputedStyle(el)
      const box = el.getBoundingClientRect()
      return style.visibility !== 'hidden' && style.display !== 'none' && box.width >= 1 && box.height >= 1
    },
  ).length

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
  // Built from here, so what is measured is what is on disk. A failure is fatal
  // rather than a fall back to whatever was there: measuring the old one is
  // exactly the outcome this exists to prevent.
  const built = spawnSync('go', ['build', '-o', 'bin/holzkube-managerd', './cmd/holzkube-managerd'], {
    cwd: '..',
    stdio: 'inherit',
  })
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
daemon.stdout.on('data', (d) => { daemonOutput += d })
daemon.stderr.on('data', (d) => { daemonOutput += d })
daemon.on('exit', (code) => {
  if (code !== null && code !== 0) {
    console.error(`the daemon exited with ${code}:\n${daemonOutput}`)
  }
})

let browser
let failures = 0
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
      const body = FIXTURES[new URL(route.request().url()).pathname]
      if (body === undefined) return route.continue()
      return route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        body: JSON.stringify(body),
      })
    })
    return context
  }

  /** Measures one route at one width, and returns how many ways it failed. */
  async function measure(page, route, width) {
    await page.goto(base + route)
    // The shell renders, then the queries land and the page grows. Measuring
    // before that is measuring an empty screen, which passes everything --
    // and a fixed wait is the flake one builds oneself: 700ms was enough on
    // a CI runner and not on the operator's Pi, where /images measured 15
    // controls instead of 32 because the Image Factory catalog had not
    // arrived. Network idle first, then a short settle for the render the
    // last response triggers.
    await page.waitForLoadState('networkidle', { timeout: 20_000 }).catch(() => {})
    await page.waitForTimeout(400)

    let found = 0
    const clipped = await page.evaluate(findClipped, width)
    let counted = ''
    if (width === TOUCH_WIDTH) {
      const wide = await page.evaluate(findWideTables, width)
      if (wide.length > 0) {
        found += 1
        console.error(`  SIDEWAYS  ${String(width).padStart(4)}px  ${route}  (${wide.length})`)
        for (const t of wide) {
          console.error(
            `              ${t.width}px wide, ${t.rows} rows, in "${t.where}" -- first row: "${t.first}"`,
          )
        }
      }
      const small = await page.evaluate(findSmallTargets, TOUCH_MIN)
      counted = `  (${await page.evaluate(countTargets)} controls, ${await page.evaluate(countItems)} items)`
      if (small.length > 0) {
        found += 1
        console.error(`  SMALL     ${String(width).padStart(4)}px  ${route}  (${small.length})`)
        for (const c of small) {
          console.error(`              <${c.tag}>${c.via} ${c.w}x${c.h} "${c.label}" .${c.cls}`)
        }
      }
    }
    // After both passes and before the verdict, so resizing for a picture
    // cannot change what was measured, and so a route WITH findings is
    // photographed too -- that is the one somebody wants to look at.
    await shoot(page, route, width)

    if (clipped.length > 0) {
      found += 1
      console.error(`  CLIPPED   ${String(width).padStart(4)}px  ${route}  (${clipped.length})`)
      for (const c of clipped.slice(0, 5)) {
        console.error(`              <${c.tag}> right=${c.right} "${c.text}" .${c.cls}`)
      }
    }
    if (found === 0) {
      // The count is printed because this guard measures what the page
      // happens to show. /images lists one control per Image Factory
      // extension, and a catalog that did not load measures as a clean run:
      // that is how 17 undersized rows passed here and failed in CI.
      console.log(`  ok        ${String(width).padStart(4)}px  ${route}${counted}`)
    }
    return found
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
    failures += await measure(page, '/setup', width)
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
    failures += await measure(page, '/login', width)
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

    for (const route of ROUTES) {
      failures += await measure(page, route, width)
    }
    await context.close()
  }
} finally {
  await browser?.close()
  daemon.kill('SIGTERM')
  await rm(dir, { recursive: true, force: true })
}

if (failures > 0) {
  console.error(
    `\n${failures} route/width combination(s) failed.\n\n` +
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
      'card, or its secondary columns fold away; it does not become a swipe.',
  )
  process.exit(1)
}
console.log(
  `\nNothing out of reach at ${WIDTHS.join('px and ')}px, ` +
    `and every control is at least ${TOUCH_MIN}px at ${TOUCH_WIDTH}px.`,
)

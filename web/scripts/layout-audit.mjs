/**
 * The layout guard: does the interface stay reachable on a phone?
 *
 * It is a script and not a vitest case because it needs the whole product --
 * the daemon serving the embedded bundle, a real session, and every route as an
 * operator actually reaches it. A component rendered in isolation cannot answer
 * this: what pushed seventeen controls off the side of /settings was the shell
 * around them, and no test of a component contains its shell.
 *
 * WHAT IT ASSERTS: nothing is clipped. An element whose right edge is past the
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
import { spawn } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { chromium } from 'playwright'

const WIDTHS = [390, 1280]
const ROUTES = [
  '/', '/nodes', '/clusters', '/config', '/jobs',
  '/provision', '/upgrades', '/images', '/audit', '/settings',
]

const BINARY = process.env.HOLZKUBE_BINARY ?? '../bin/holzkube-managerd'

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

  const setup = await fetch(`${base}/api/v1/setup`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Holzkube-Manager-CSRF': '1' },
    body: JSON.stringify({ username: USER, password: PASSWORD }),
  })
  if (!setup.ok) {
    throw new Error(`could not create the audit account: ${setup.status} ${await setup.text()}`)
  }

  browser = await chromium.launch(BROWSER ? { executablePath: BROWSER } : {})

  for (const width of WIDTHS) {
    const context = await browser.newContext({ viewport: { width, height: 844 } })
    const page = await context.newPage()

    await page.goto(`${base}/login`)
    await page.fill('#login-username', USER)
    await page.fill('#login-password', PASSWORD)
    await page.click('button[type="submit"]')
    await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 20_000 })

    for (const route of ROUTES) {
      await page.goto(base + route)
      // The shell renders, then the queries land and the page grows. Measuring
      // before that is measuring an empty screen, which passes everything.
      await page.waitForTimeout(700)

      const clipped = await page.evaluate(findClipped, width)
      if (clipped.length === 0) {
        console.log(`  ok        ${String(width).padStart(4)}px  ${route}`)
        continue
      }

      failures += 1
      console.error(`  CLIPPED   ${String(width).padStart(4)}px  ${route}  (${clipped.length})`)
      for (const c of clipped.slice(0, 5)) {
        console.error(`              <${c.tag}> right=${c.right} "${c.text}" .${c.cls}`)
      }
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
    `\n${failures} route/width combination(s) put content out of reach.\n\n` +
      'An element past the right edge with nothing to scroll is not narrow, it is gone: ' +
      'no swipe brings it back and no scrollbar says it is there. That is how the ' +
      '"New account" form and the change-password button left /settings on a phone ' +
      'without anything appearing to be wrong.\n\n' +
      'Either let it wrap, or put it in a container that scrolls sideways.',
  )
  process.exit(1)
}
console.log(`\nNothing out of reach at ${WIDTHS.join('px and ')}px.`)

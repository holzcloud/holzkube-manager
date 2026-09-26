/**
 * Renders the pictures the README shows -- the banner and social card in
 * docs/brand, the screenshots in docs/screenshots -- and the app's home-screen
 * icons in web/public.
 *
 *   ./bin/task build && node web/scripts/readme-images.mjs
 *
 * The screens are the product's own -- the built daemon serving the embedded
 * bundle, a real session -- with every GET answered from web/fixtures/demo.json,
 * the same invented homelab the layout guard renders. The values that move
 * (usage, temperatures, the 24-hour history) are generated here so the charts
 * have a day behind them. Nothing in these pictures comes from a real cluster:
 * the repository is public (see CLAUDE.md), and a screenshot is the easiest
 * place for a real host name to slip back in.
 */
import { spawn } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

const root = fileURLToPath(new URL('../../', import.meta.url))
const shots = join(root, 'docs/screenshots')
const brand = join(root, 'docs/brand')
const F = JSON.parse(readFileSync(join(root, 'web/fixtures/demo.json'), 'utf8'))
const executablePath = process.env.CHROMIUM ?? undefined

const browser = await chromium.launch({ executablePath })

// --- brand ------------------------------------------------------------------

const mark = readFileSync(join(brand, 'holzkube-mark.svg'), 'utf8').replace(/<!--[\s\S]*?-->/, '')
// Inlined: a page made with setContent has no origin that may read file://.
const font = `data:font/woff2;base64,${readFileSync(
  join(root, 'web/node_modules/@fontsource-variable/manrope/files/manrope-latin-wght-normal.woff2'),
).toString('base64')}`
const ground = `radial-gradient(55% 130% at 10% 25%, rgb(138 83 38 / .6), transparent 70%),
  radial-gradient(45% 100% at 92% 85%, rgb(107 63 122 / .32), transparent 70%), #0a0705`

async function card(file, width, height, body) {
  const page = await browser.newPage({ viewport: { width, height }, deviceScaleFactor: 2 })
  await page.setContent(`<!doctype html><meta charset="utf-8"><style>
    @font-face { font-family: Manrope; src: url(${font}) format("woff2"); font-weight: 200 800 }
    html, body { margin: 0; width: ${width}px; height: ${height}px; overflow: hidden }
    body { font-family: Manrope, sans-serif; color: #f6efe6; background: ${ground} }
    .t { font-weight: 760; letter-spacing: -.035em; line-height: 1 }
    .s { color: #d8c8b4 }
    .pills { display: flex; gap: 10px; flex-wrap: wrap }
    .pills span { font-size: 15px; font-weight: 600; color: #f0ae5f; padding: 6px 13px;
      border: 1px solid rgb(240 174 95 / .35); border-radius: 999px; background: rgb(240 174 95 / .08) }
  </style>${body}`)
  await page.evaluate(() => document.fonts.ready)
  await page.screenshot({ path: join(brand, file) })
  await page.close()
}

await card(
  'banner.png',
  1280,
  320,
  `<div style="display:flex;align-items:center;gap:44px;height:100%;padding:0 72px">
    <div style="width:168px;height:168px;flex:none">${mark.replace('width="64" height="64"', 'width="168" height="168"')}</div>
    <div>
      <div class="t" style="font-size:64px">holzkube-manager</div>
      <div class="s" style="font-size:22px;margin:14px 0 22px">Self-hosted management for Talos Linux and Kubernetes.<br>One binary, running beside your cluster.</div>
      <div class="pills"><span>Talos Linux</span><span>Kubernetes</span><span>Live hardware</span><span>Single binary</span><span>arm64 · amd64</span></div>
    </div>
  </div>`,
)
await card(
  'social.png',
  1280,
  640,
  `<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;height:100%;gap:34px">
    <div style="width:220px;height:220px">${mark.replace('width="64" height="64"', 'width="220" height="220"')}</div>
    <div class="t" style="font-size:76px">holzkube-manager</div>
    <div class="s" style="font-size:26px">Talos Linux and Kubernetes, managed from one binary.</div>
  </div>`,
)

// The home-screen icon: the mark on the ground colour, opaque, because iOS
// fills a transparent corner with black and Android masks to its own shape. The
// mark stays inside the middle 64%, the safe zone of a maskable icon.
const favicon = readFileSync(join(root, 'web/public/favicon.svg'), 'utf8').replace(/<!--[\s\S]*?-->/, '')
for (const [file, size] of [
  ['apple-touch-icon.png', 180],
  ['icon-192.png', 192],
  ['icon-512.png', 512],
]) {
  const page = await browser.newPage({ viewport: { width: size, height: size }, deviceScaleFactor: 1 })
  const inner = Math.round(size * 0.64)
  await page.setContent(`<!doctype html><style>html,body{margin:0;background:#150e08}
    div{width:${size}px;height:${size}px;display:grid;place-items:center}</style>
    <div>${favicon.replace('width="64" height="64"', `width="${inner}" height="${inner}"`)}</div>`)
  await page.screenshot({ path: join(root, 'web/public', file) })
  await page.close()
}

// --- screenshots -------------------------------------------------------------

const dir = await mkdtemp(join(tmpdir(), 'readme-images-'))
const port = 18600
const base = `http://127.0.0.1:${port}`
const daemon = spawn(
  join(root, 'bin/holzkube-managerd'),
  ['--insecure-http', '--listen', `127.0.0.1:${port}`, '--data-dir', dir, '--log-level', 'error'],
  { stdio: 'ignore' },
)
for (let i = 0; i < 100; i++) {
  try {
    if ((await fetch(`${base}/api/v1/system/status`)).ok) break
  } catch {}
  await new Promise((r) => setTimeout(r, 200))
}
const password = 'a-long-enough-passphrase-for-the-readme-1'
await fetch(`${base}/api/v1/setup`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json', 'X-Holzkube-Manager-CSRF': '1' },
  body: JSON.stringify({ username: 'operator', password }),
})

// A deterministic wobble, so two runs draw the same day.
const wave = (b, a, i, ph = 0) =>
  Math.max(0, b + a * Math.sin(i / 90 + ph) + a * 0.5 * Math.sin(i / 7 + ph * 2) + a * 0.2 * Math.sin(i * 1.7 + ph))

function history(url) {
  const range = url.searchParams.get('range') ?? '1h'
  const step = range === '1h' ? 15_000 : 60_000
  const n = range === '1h' ? 240 : range === '6h' ? 360 : 1440
  const end = Date.now() - 5_000
  const mk = (f) => Array.from({ length: n }, (_, i) => [end - (n - 1 - i) * step, Math.round(f(i) * 10) / 10])
  const series = url.pathname.includes('/apps/')
    ? { cpu: mk((i) => wave(1300, 400, i)), memory: mk((i) => wave(1.9 * 2 ** 30, 1e8, i, 1)) }
    : Object.fromEntries([
        ['cpu', mk((i) => wave(22, 9, i))],
        ['memory', mk((i) => wave(36, 2, i, 1))],
        ['rx', mk((i) => wave(1.1e6, 6e5, i, 2))],
        ['tx', mk((i) => wave(2e5, 1e5, i, 3))],
        ['read', mk((i) => wave(2e6, 1.5e6, i, 4))],
        ['write', mk((i) => wave(8e5, 5e5, i, 5))],
        ...[0, 1, 2, 3, 4, 5].map((c) => [`core:${c}`, mk((i) => (c === 2 ? Math.min(100, wave(80, 8, i)) : wave(6, 4, i, c)))]),
        ...F['/api/v1/machines/m-cp-1/hardware'].temperatures.map((t) => [
          `temp:${t.chip}/${t.label}`,
          mk((i) => wave(t.celsius, 2, i)),
        ]),
      ])
  return { range, step_seconds: step / 1000, from: '', to: '', series }
}

function live(path, body) {
  const now = new Date().toISOString()
  // The fixture's second cluster has a certificate running out, which the
  // layout guard needs and a picture of every other screen does not.
  if (path === '/api/v1/clusters') {
    for (const c of body.clusters) {
      Object.assign(c, { certificate_warning: '', certificate_urgency: 'none' })
      if (c.client_cert_days_left < 60) Object.assign(c, { client_cert_days_left: 64, client_cert_not_after: '2026-11-22T08:00:00Z' })
    }
  }
  if (path.endsWith('/hardware')) body.observed_at = now
  if (path.includes('/kubernetes/apps')) body.collected_at = now
  if (path.endsWith('/wall')) {
    body.generated_at = now
    const end = Date.now()
    const day = (b, a, ph) =>
      Array.from({ length: 1440 }, (_, i) => [end - (1439 - i) * 60_000, Math.round(wave(b, a, i, ph) * 10) / 10])
    const hour = (b, a) => Array.from({ length: 240 }, (_, i) => [end - (239 - i) * 15_000, wave(b, a, i * 4)])
    body.trends = {
      cluster: { cpu: day(26, 9, 0), memory: day(47, 3, 1) },
      nodes: Object.fromEntries(body.nodes.map((n, k) => [n.name, hour(15 + k * 18, 5)])),
    }
  }
  return body
}

async function context(width, height, scale) {
  const c = await browser.newContext({ viewport: { width, height }, deviceScaleFactor: scale })
  await c.addInitScript(() => localStorage.setItem('holzkube-manager.theme', 'dark'))
  await c.route('**/api/v1/**', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/history')) {
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(history(url)) })
    }
    const body = F[url.pathname]
    if (body === undefined) return route.continue()
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(live(url.pathname, structuredClone(body))),
    })
  })
  const page = await c.newPage()
  await page.goto(`${base}/login`)
  await page.fill('#login-username', 'operator')
  await page.fill('#login-password', password)
  await page.click('button[type="submit"]')
  await page.waitForURL((u) => !u.pathname.startsWith('/login'))
  return { c, page }
}

async function shoot(page, route, file, { range, wait = 2500 } = {}) {
  await page.goto(base + route)
  await page.waitForTimeout(wait)
  if (range) {
    await page.getByRole('button', { name: range }).first().click()
    await page.waitForTimeout(2000)
  }
  await page.mouse.move(0, 0)
  await page.screenshot({ path: join(shots, file) })
}

const desk = await context(1440, 900, 1)
await shoot(desk.page, '/clusters', 'clusters.png')
await shoot(desk.page, '/nodes/m-cp-1', 'node-hardware.png', { range: '24 h' })
await shoot(desk.page, '/kubernetes/apps', 'apps.png')
await shoot(desk.page, '/kubernetes/apps/media/Deployment/jellyfin', 'app-detail.png', { range: '24 h' })
await shoot(desk.page, '/kubernetes', 'kubernetes.png')
await desk.c.close()

const wall = await context(1600, 900, 1)
await shoot(wall.page, '/wall', 'wall.png', { wait: 4000 })
await wall.c.close()

// Three phone screens side by side, as one picture.
const phone = await context(390, 844, 2)
const phones = []
for (const route of ['/nodes/m-cp-1', '/kubernetes/apps', '/clusters']) {
  await phone.page.goto(base + route)
  await phone.page.waitForTimeout(3000)
  phones.push((await phone.page.screenshot()).toString('base64'))
}
await phone.c.close()
const strip = await browser.newPage({ viewport: { width: 1400, height: 920 }, deviceScaleFactor: 1 })
await strip.setContent(`<!doctype html><style>
  html, body { margin: 0; background: transparent }
  div { display: flex; gap: 40px; justify-content: center; padding: 18px 0 }
  img { width: 390px; height: 844px; border-radius: 34px; border: 10px solid #1d140c;
    box-shadow: 0 18px 40px rgb(0 0 0 / .35) }
</style><div>${phones.map((b) => `<img src="data:image/png;base64,${b}">`).join('')}</div>`)
await strip.screenshot({ path: join(shots, 'phone.png'), omitBackground: true })

await browser.close()
daemon.kill()
await rm(dir, { recursive: true, force: true })
console.log('wrote docs/brand/{banner,social}.png, web/public/*.png and docs/screenshots/*.png')

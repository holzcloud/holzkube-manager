/**
 * Compares two desk-width layout dumps: did any control move or change size?
 *
 *   node web/scripts/layout-dump-compare.mjs <before.jsonl> <after.jsonl>
 *
 * WHAT IT IS FOR: D-08, "the desk unchanged, measured once". Phase 14 raises
 * tap targets below `md` only, and `max-md:` holds the desk by construction --
 * but a class written without its prefix is exactly the defect a reviewer
 * misses. So the claim is measured instead of trusted: `LAYOUT_DUMP=<file>
 * ./bin/task test:layout` writes one JSON line per control at 1280px (see
 * layout-audit.mjs), once before the first class change and once after the
 * last, and this judges the two.
 *
 * WHAT IT COMPARES: the key is route · opener · index · tag, where index is the
 * control's place in DOM order within its scope, and a control's x, y, width
 * and height are held equal. The name is printed with a difference and never
 * compared: relative times and live values change it between two runs of the
 * same tree (Pitfall 9), and a name is not a layout.
 *
 * WHAT FAILS IT: any box that differs, a control present in only one dump, a
 * key that appears twice in one dump, a line that is not a control, and an
 * empty dump on either side -- a dump that measured nothing compares equal to
 * anything, and "0 differ" out of 0 is not a result.
 *
 * IT IS NOT A BASELINE. Nothing it reads is checked in (D-08 forbids that, and
 * the audit refuses a dump path inside the repository): a baseline in the
 * repository would be regenerated the first time it disagreed, and then it
 * measures nothing. Both dumps are taken on one machine from one fixture, and
 * the instrument's noise -- two dumps of an unchanged tree -- is measured at 0
 * before either is used to judge anything.
 */
import { readFileSync } from 'node:fs'

const FIELDS = ['x', 'y', 'width', 'height']

/** Reads a dump into a Map by key, or returns the reason it cannot be read. */
function load(file) {
  let text
  try {
    text = readFileSync(file, 'utf8')
  } catch (error) {
    return { error: `${file} cannot be read: ${error.message}` }
  }
  const lines = text.split('\n').filter((l) => l.trim() !== '')
  if (lines.length === 0) {
    return {
      error: `${file} has no controls: a dump that measured nothing compares equal to anything`,
    }
  }
  const controls = new Map()
  for (const [i, line] of lines.entries()) {
    let c
    try {
      c = JSON.parse(line)
    } catch {
      return { error: `${file}:${i + 1} is not JSON` }
    }
    const shaped =
      typeof c.route === 'string' &&
      typeof c.opener === 'string' &&
      Number.isInteger(c.index) &&
      typeof c.tag === 'string' &&
      FIELDS.every((f) => Number.isInteger(c[f]))
    if (!shaped) {
      return {
        error: `${file}:${i + 1} is not a control (route, opener, index, tag, x, y, width, height)`,
      }
    }
    const key = `${c.route} · ${c.opener === '' ? '(route)' : c.opener} · #${c.index} · <${c.tag}>`
    if (controls.has(key)) {
      return { error: `${file}:${i + 1} repeats ${key}: the index is not unique within its scope` }
    }
    controls.set(key, c)
  }
  return { controls }
}

const box = (c) => `${c.x},${c.y} ${c.width}x${c.height}`

const [beforeFile, afterFile] = process.argv.slice(2)
if (beforeFile === undefined || afterFile === undefined) {
  console.error('usage: node web/scripts/layout-dump-compare.mjs <before.jsonl> <after.jsonl>')
  process.exit(1)
}

const before = load(beforeFile)
const after = load(afterFile)
const unreadable = [before.error, after.error].filter((e) => e !== undefined)
if (unreadable.length > 0) {
  for (const e of unreadable) console.error(e)
  process.exit(1)
}

const differences = []
for (const [key, b] of before.controls) {
  const a = after.controls.get(key)
  if (a === undefined) {
    differences.push(`  ONLY BEFORE  ${key}  ${box(b)} "${b.name ?? ''}"`)
    continue
  }
  if (FIELDS.some((f) => a[f] !== b[f])) {
    differences.push(
      `  MOVED        ${key}\n` +
        `                 before ${box(b)} "${b.name ?? ''}"\n` +
        `                 after  ${box(a)} "${a.name ?? ''}"`,
    )
  }
}
for (const [key, a] of after.controls) {
  if (!before.controls.has(key)) {
    differences.push(`  ONLY AFTER   ${key}  ${box(a)} "${a.name ?? ''}"`)
  }
}

const compared = new Set([...before.controls.keys(), ...after.controls.keys()]).size
console.log(`compared ${compared} controls, ${differences.length} differ`)
for (const d of differences) console.log(d)
process.exit(differences.length > 0 ? 1 : 0)

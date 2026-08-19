import { gzipSync } from 'node:zlib'
import { readdir, readFile, stat } from 'node:fs/promises'
import path from 'node:path'

const root = path.resolve(process.argv[2] ?? 'dist')
const initialJsBudget = 200 * 1024
const cssBudget = 50 * 1024

async function filesUnder(directory) {
  const files = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const absolute = path.join(directory, entry.name)
    if (entry.isDirectory()) files.push(...await filesUnder(absolute))
    else files.push(absolute)
  }
  return files.sort()
}

const index = await readFile(path.join(root, 'index.html'), 'utf8')
const initialSources = [...index.matchAll(/<(?:script|link)[^>]+(?:src|href)="([^"]+)"/g)]
  .map((match) => match[1])
  .filter((name) => name.endsWith('.js'))
const all = await filesUnder(root)
const css = all.filter((name) => name.endsWith('.css'))

if (initialSources.length === 0) throw new Error('index.html has no initial JavaScript entry')
let initialGzip = 0
for (const source of initialSources) {
  const bytes = await readFile(path.join(root, source.replace(/^\//, '')))
  initialGzip += gzipSync(bytes, { level: 9 }).byteLength
}
let cssGzip = 0
for (const file of css) cssGzip += gzipSync(await readFile(file), { level: 9 }).byteLength

for (const file of all) {
  const info = await stat(file)
  if (info.size === 0) throw new Error(`empty bundle file: ${path.relative(root, file)}`)
}
if (initialGzip > initialJsBudget) throw new Error(`initial JavaScript is ${initialGzip} gzip bytes; budget is ${initialJsBudget}`)
if (cssGzip > cssBudget) throw new Error(`CSS is ${cssGzip} gzip bytes; budget is ${cssBudget}`)

console.log(JSON.stringify({ initial_js_gzip_bytes: initialGzip, css_gzip_bytes: cssGzip, files: all.length }))

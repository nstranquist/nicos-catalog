import { spawnSync } from 'node:child_process'
import { mkdtemp, readdir, readFile, rm } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const explorer = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const root = path.dirname(explorer)
const expected = path.join(explorer, 'dist')
const temporary = await mkdtemp(path.join(os.tmpdir(), 'nicos-catalog-explorer-embed-'))

try {
  const result = spawnSync('corepack', ['pnpm@11.13.0', '--dir', 'explorer', 'build'], {
    cwd: root,
    env: { ...process.env, EXPLORER_OUT_DIR: temporary },
    encoding: 'utf8',
  })
  if (result.status !== 0) {
    process.stderr.write(result.stdout ?? '')
    process.stderr.write(result.stderr ?? '')
    throw new Error(`Explorer rebuild failed with status ${result.status ?? 'unknown'}`)
  }
  const committedFiles = await filesUnder(expected)
  const rebuiltFiles = await filesUnder(temporary)
  const committedNames = committedFiles.map((name) => path.relative(expected, name))
  const rebuiltNames = rebuiltFiles.map((name) => path.relative(temporary, name))
  if (JSON.stringify(committedNames) !== JSON.stringify(rebuiltNames)) {
    throw new Error(`embedded file list differs\ncommitted: ${committedNames.join(', ')}\nrebuilt: ${rebuiltNames.join(', ')}`)
  }
  for (let index = 0; index < committedFiles.length; index += 1) {
    const committed = await readFile(committedFiles[index])
    const rebuilt = await readFile(rebuiltFiles[index])
    if (!committed.equals(rebuilt)) throw new Error(`embedded bytes differ: ${committedNames[index]}`)
  }
  console.log(JSON.stringify({ embedded_bundle: 'byte-identical', files: committedFiles.length }))
} finally {
  await rm(temporary, { recursive: true, force: true })
}

async function filesUnder(directory) {
  const files = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const absolute = path.join(directory, entry.name)
    if (entry.isDirectory()) files.push(...await filesUnder(absolute))
    else files.push(absolute)
  }
  return files.sort()
}

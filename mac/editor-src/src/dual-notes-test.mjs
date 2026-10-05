import { build } from 'esbuild'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { spawnSync } from 'node:child_process'

const temporary = await mkdtemp(path.join(tmpdir(), 'panel-dual-notes-test-'))
try {
  const outfile = path.join(temporary, 'dual-notes-core.test.mjs')
  await build({ entryPoints: ['src/dual-notes-core.test.mjs'], bundle: true, platform: 'node', format: 'esm', outfile })
  const run = spawnSync(process.execPath, ['--test', outfile], { stdio: 'inherit', env: { ...process.env, TZ: 'America/Los_Angeles' } })
  process.exitCode = run.status ?? 1
} finally { await rm(temporary, { recursive: true, force: true }) }

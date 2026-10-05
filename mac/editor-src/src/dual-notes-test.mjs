import { build } from 'esbuild'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { spawnSync } from 'node:child_process'

const temporary = await mkdtemp(path.join(tmpdir(), 'panel-dual-notes-test-'))
try {
  const entries = ['src/dual-notes-core.test.mjs', 'src/dual-notes-code.test.mjs', 'src/dual-notes-export.test.mjs']
  await build({ entryPoints: entries, bundle: true, platform: 'node', format: 'esm', outdir: temporary, outExtension: { '.js': '.mjs' } })
  const run = spawnSync(process.execPath, ['--test', ...entries.map(entry => path.join(temporary, path.basename(entry)))], { stdio: 'inherit', env: { ...process.env, TZ: 'America/Los_Angeles' } })
  process.exitCode = run.status ?? 1
} finally { await rm(temporary, { recursive: true, force: true }) }

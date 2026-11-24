import { cpSync, existsSync, mkdirSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'

const root = fileURLToPath(new URL('../', import.meta.url))
const web = join(root, 'web')
const server = join(root, 'server')
const output = join(root, 'dist')
const embedded = join(server, 'internal', 'webassets', 'dist')
const linux = process.argv.includes('--linux')

function run(command, args, options) {
  const child = spawnSync(command, args, { stdio: 'inherit', ...options })
  if (child.error) throw child.error
  if (child.status !== 0) process.exit(child.status ?? 1)
}

run('npm', ['ci'], { cwd: web, shell: process.platform === 'win32' })
run('npm', ['run', 'build'], { cwd: web, shell: process.platform === 'win32' })
if (realpathSync(embedded) !== join(realpathSync(root), 'server', 'internal', 'webassets', 'dist')) {
  throw new Error('Embedded assets must be inside the source tree')
}
const generated = join(embedded, 'assets')
if (existsSync(generated)) {
  if (realpathSync(generated) !== generated) throw new Error('Refusing to replace linked build assets')
  rmSync(generated, { recursive: true })
}
cpSync(join(web, 'dist'), embedded, { recursive: true })
mkdirSync(output, { recursive: true })
const git = spawnSync('git', ['rev-parse', '--short=12', 'HEAD'], { cwd: root, encoding: 'utf8' })
const status = spawnSync('git', ['status', '--porcelain'], { cwd: root, encoding: 'utf8' })
const commit = git.status === 0 ? git.stdout.trim() : 'source'
const dirty = status.status === 0 && status.stdout.trim() !== ''
const time = new Date().toISOString().replace(/\.\d+Z$/, 'Z')
const build = commit + (dirty ? '-dirty' : '') + '+' + time
const file = linux || process.platform !== 'win32' ? 'gandengyan' : 'gandengyan.exe'
const binary = join(output, file)
run('go', ['build', '-trimpath', '-ldflags', '-s -w -X main.build=' + build, '-o', binary, './cmd/server'], {
  cwd: server,
  env: { ...process.env, CGO_ENABLED: '0', ...(linux ? { GOOS: 'linux', GOARCH: 'amd64' } : {}) },
})
const checksum = createHash('sha256').update(readFileSync(binary)).digest('hex')
writeFileSync(binary + '.sha256', checksum + '  ' + file + '\n')
writeFileSync(binary + '.json', JSON.stringify({ build, commit, dirty, builtAt: time, protocolVersion: 1, rulesVersion: '1.3', sha256: checksum }, null, 2) + '\n')
console.log(binary)

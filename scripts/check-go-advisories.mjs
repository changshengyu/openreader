#!/usr/bin/env node
import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {mkdtemp, readFile, writeFile} from 'node:fs/promises'
import {tmpdir} from 'node:os'
import {dirname, join, resolve} from 'node:path'
import {fileURLToPath} from 'node:url'

const scanner = 'golang.org/x/vuln/cmd/govulncheck@v1.9.0'
const openpgp = 'golang.org/x/crypto/openpgp'
const day = 86_400_000
const isOpenPGP = path => path === openpgp || path.startsWith(`${openpgp}/`)

export function parseMessages(raw) {
  assert.equal(typeof raw, 'string')
  const result = []
  let start = -1, depth = 0, quoted = false, escaped = false
  for (let i = 0; i < raw.length; i++) {
    const char = raw[i]
    if (start < 0) {
      if (/\s/.test(char)) continue
      assert.equal(char, '{', 'expected JSON object')
      start = i
    }
    if (quoted) {
      if (escaped) escaped = false
      else if (char === '\\') escaped = true
      else if (char === '"') quoted = false
    } else if (char === '"') quoted = true
    else if (char === '{') depth++
    else if (char === '}' && --depth === 0) {
      result.push(JSON.parse(raw.slice(start, i + 1)))
      start = -1
    }
  }
  assert.ok(result.length > 0 && start === -1 && depth === 0 && !quoted, 'empty or incomplete JSON stream')
  return result
}

export function evaluateSourceScan(raw, evidence, {now = Date.now()} = {}) {
  const messages = parseMessages(raw)
  const grouped = {config:[], SBOM:[], progress:[], osv:[], finding:[]}
  for (const message of messages) {
    const keys = Object.keys(message)
    assert.ok(keys.length === 1 && Object.hasOwn(grouped, keys[0]), 'unknown or ambiguous scanner message')
    const value = message[keys[0]]
    assert.ok(value && typeof value === 'object' && !Array.isArray(value), 'invalid message payload')
    grouped[keys[0]].push(value)
  }
  assert.equal(grouped.config.length, 1, 'exactly one config required')
  assert.equal(grouped.SBOM.length, 1, 'exactly one SBOM required')
  const config = grouped.config[0], sbom = grouped.SBOM[0]
  assert.equal(config.protocol_version, 'v1.0.0')
  assert.equal(config.scanner_name, 'govulncheck')
  assert.equal(config.scanner_version, 'v1.9.0')
  assert.equal(config.db, 'https://vuln.go.dev')
  assert.equal(config.scan_mode, 'source', 'binary precision is not source evidence')
  assert.equal(config.scan_level, 'symbol')
  assert.match(evidence.goVersion, /^go\d+\.\d+\.\d+$/)
  assert.equal(config.go_version, evidence.goVersion)
  assert.equal(sbom.go_version, evidence.goVersion)
  const dbDate = Date.parse(config.db_last_modified)
  assert.ok(Number.isFinite(now) && Number.isFinite(dbDate) && now - dbDate <= 14 * day
    && dbDate - now <= day, 'invalid, stale or future vulnerability DB')
  assert.ok(Array.isArray(sbom.roots) && sbom.roots.includes('openreader/backend'), 'application root missing')
  assert.ok(Array.isArray(sbom.modules) && sbom.modules.length > 0, 'modules missing')
  const modules = new Map()
  for (const module of sbom.modules) {
    assert.ok(typeof module.path === 'string' && module.path && !modules.has(module.path), 'invalid/duplicate module')
    modules.set(module.path, module.version)
  }
  assert.ok(modules.has('openreader/backend'))
  assert.equal(modules.get('golang.org/x/crypto'), evidence.cryptoVersion)
  // Tagged releases only: no pseudo-version, pre-release or +incompatible exception remains.
  assert.match(evidence.cryptoVersion, /^v\d+\.\d+\.\d+$/, 'x/crypto must be a tagged release')
  assert.ok(['darwin','linux'].includes(evidence.goos), 'unsupported host OS')
  assert.ok(['amd64','arm64'].includes(evidence.goarch), 'unsupported host architecture')
  assert.equal(evidence.cgoEnabled, '1', 'CGO source selection required')
  for (const target of ['host','linux/amd64','linux/arm64']) {
    const packages = evidence.graphs?.[target]
    assert.ok(Array.isArray(packages) && packages.length > 0
      && packages.every(path => typeof path === 'string' && path.length > 0), `invalid ${target} import graph`)
    assert.ok(packages.includes('openreader/backend') && packages.includes('golang.org/x/crypto/bcrypt'),
      `missing main/bcrypt in ${target} import graph`)
    assert.ok(!packages.some(isOpenPGP), `OpenPGP imported in ${target} import graph`)
  }
  const osvs = new Map()
  for (const osv of grouped.osv) {
    assert.ok(typeof osv.id === 'string' && osv.id, 'invalid OSV')
    if (osvs.has(osv.id)) assert.deepEqual(osv, osvs.get(osv.id), 'conflicting repeated OSV')
    osvs.set(osv.id, osv)
  }
  // No not-applicable exceptions remain: every finding blocks, whatever its precision, fix or OSV metadata.
  assert.equal(grouped.finding.length, 0,
    `advisory findings block release: ${[...new Set(grouped.finding.map(finding => finding.osv))].join(', ')}`)
  return {status:'pass', scanMode:'source', scanLevel:'symbol', goVersion:evidence.goVersion,
    goos:evidence.goos, goarch:evidence.goarch, cgoEnabled:evidence.cgoEnabled,
    dbLastModified:config.db_last_modified, advisories:osvs.size, findings:grouped.finding.length,
    crypto:{version:evidence.cryptoVersion},
    limitations:['C/OS/Node/frontend not scanned', 'target import graphs are not cross-platform binary scans']}
}

// Each tool owns a POSIX process group. On timeout/output overflow, kill that group and await close,
// including a go-run scanner descendant; killing only the Go launcher can leave work behind.
export function runTool(file, args, {cwd, env = process.env, timeout = 600_000, maxBytes = 16 << 20, captureOutput = false} = {}) {
  assert.ok(process.platform !== 'win32', 'process-group lifecycle requires POSIX')
  return new Promise((resolvePromise, reject) => {
    const child = spawn(file, args, {cwd, env, detached:true, stdio:['ignore','pipe','pipe']})
    const chunks = {stdout:[], stderr:[]}
    let bytes = 0, failure
    const stop = error => {
      if (failure) return
      failure = error
      if (child.pid && child.exitCode === null && child.signalCode === null) {
        try {process.kill(-child.pid, 'SIGKILL')} catch (killError) {
          // The child can exit while buffered output is still arriving. Keep the original
          // failure, never report success, and await close rather than replacing its cause.
          if (killError.code !== 'ESRCH') failure.cause = killError
        }
      }
    }
    const interrupt = () => stop(new Error('tool interrupted'))
    process.once('SIGINT', interrupt)
    process.once('SIGTERM', interrupt)
    const timer = setTimeout(() => stop(new Error('tool timeout')), timeout)
    for (const [stream, name] of [[child.stdout,'stdout'],[child.stderr,'stderr']]) {
      stream.on('data', chunk => {
        bytes += chunk.length
        if (bytes > maxBytes) {stop(new Error('tool output limit exceeded')); return}
        chunks[name].push(chunk)
      })
    }
    child.on('error', error => {failure ||= error})
    child.on('close', (code, signal) => {
      clearTimeout(timer)
      process.removeListener('SIGINT', interrupt)
      process.removeListener('SIGTERM', interrupt)
      const stdout = Buffer.concat(chunks.stdout).toString('utf8')
      const stderr = Buffer.concat(chunks.stderr).toString('utf8')
      if (!failure && (code !== 0 || signal)) failure = new Error(`tool failed: exit ${code}, signal ${signal}`)
      if (failure) {
        failure.stdout = stdout
        failure.stderr = stderr
        failure.exitCode = code
        failure.signal = signal
        reject(failure)
      } else resolvePromise(captureOutput ? {stdout,stderr,exitCode:code,signal} : stdout)
    })
  })
}

async function main() {
  assert.equal(process.argv.length, 2, 'no external scan/evidence inputs or bypass flags accepted')
  const backend = join(dirname(fileURLToPath(import.meta.url)), '..', 'backend')
  const output = await mkdtemp(join(tmpdir(), 'openreader-go-advisories-'))
  console.log(`Go advisory evidence: ${output}`)
  let raw = ''
  try {
    const goEnv = JSON.parse(await runTool('go', ['env','-json','GOVERSION','GOOS','GOARCH','CGO_ENABLED','GOMOD'], {cwd:backend}))
    assert.equal(resolve(goEnv.GOMOD), resolve(backend, 'go.mod'), 'unexpected module cwd')
    const mod = await readFile(join(backend, 'go.mod'), 'utf8')
    const minimum = mod.match(/^go (\d+)\.(\d+)\.(\d+)$/m)
    assert.ok(minimum, 'explicit minimum patch required')
    const actual = goEnv.GOVERSION.match(/^go(\d+)\.(\d+)\.(\d+)$/)
    assert.ok(actual)
    const number = parts => parts.slice(1).reduce((n, part) => n * 1000 + Number(part), 0)
    assert.ok(number(actual) >= number(minimum), 'Go below project minimum')
    const cryptoVersion = (await runTool('go',['list','-m','-f','{{.Version}}','golang.org/x/crypto'],{cwd:backend})).trim()
    const graphs = {}
    for (const target of ['host','linux/amd64','linux/arm64']) {
      const env = target === 'host' ? process.env : {...process.env, GOOS:'linux', GOARCH:target.split('/')[1], CGO_ENABLED:'1'}
      const text = await runTool('go',['list','-deps',...(target === 'host' ? ['./...'] : ['.'])],{cwd:backend,env})
      graphs[target] = text.trim().split('\n')
      await writeFile(join(output, `${target.replace('/','-')}-packages.txt`), text)
    }
    const evidence = {goVersion:goEnv.GOVERSION, goos:goEnv.GOOS, goarch:goEnv.GOARCH,
      cgoEnabled:goEnv.CGO_ENABLED, cryptoVersion, graphs}
    await writeFile(join(output, 'environment.json'), JSON.stringify(evidence,null,2))
    try {raw = await runTool('go',['run',scanner,'-json','./...'],{cwd:backend})}
    catch (error) {
      await writeFile(join(output, 'scanner.json'), error.stdout || '')
      await writeFile(join(output, 'scanner.stderr'), error.stderr || '')
      throw error
    }
    await writeFile(join(output, 'scanner.json'), raw)
    const summary = evaluateSourceScan(raw, evidence)
    await writeFile(join(output, 'summary.json'), JSON.stringify(summary,null,2))
    console.log(JSON.stringify(summary,null,2))
  } catch (error) {
    await writeFile(join(output,'failure.txt'), String(error))
    throw error
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(error => {console.error(`Go advisory gate failed: ${error.message}`); process.exitCode = 1})
}

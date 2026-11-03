import assert from 'node:assert/strict'
import test from 'node:test'
import {readFile} from 'node:fs/promises'
import {evaluateSourceScan, parseMessages, runTool} from './check-go-advisories.mjs'

const now = Date.parse('2026-10-16T03:00:00Z')
// x/crypto 2f1f834330b2 is v0.58.0 with openpgp/ deleted; GO-2026-5932 names it as the fix since 2026-11-02.
const pin = 'v0.58.1-0.20261009181718-2f1f834330b2'
const openpgpImports = ['', '/packet', '/armor', '/clearsign', '/errors', '/elgamal', '/s2k']
  .map(suffix => ({path:`golang.org/x/crypto/openpgp${suffix}`}))
function fixture() {
  const packages = ['openreader/backend', 'golang.org/x/crypto/bcrypt']
  return {
    evidence: {goVersion:'go1.26.9', goos:'darwin', goarch:'arm64', cgoEnabled:'1',
      cryptoVersion:'v0.57.0', graphs:{host:packages, 'linux/amd64':packages, 'linux/arm64':packages}},
    messages: [
      {config:{protocol_version:'v1.0.0', scanner_name:'govulncheck', scanner_version:'v1.9.0',
        db:'https://vuln.go.dev', db_last_modified:'2026-10-08T22:31:09Z', go_version:'go1.26.9',
        scan_level:'symbol', scan_mode:'source'}},
      {SBOM:{go_version:'go1.26.9', roots:['openreader/backend'], modules:[
        {path:'openreader/backend'}, {path:'golang.org/x/crypto', version:'v0.57.0'}]}},
      // The record as modified on 2026-11-02: fixed by the pin, plus a standalone deprecated module.
      {osv:{id:'GO-2026-5932', affected:[
        {package:{ecosystem:'Go', name:'golang.org/x/crypto'}, ranges:[{type:'SEMVER',events:[
          {introduced:'0'}, {fixed:pin.slice(1)}]}], ecosystem_specific:{imports:openpgpImports}},
        {package:{ecosystem:'Go', name:'golang.org/x/crypto/openpgp'}, ranges:[{type:'SEMVER',events:[
          {introduced:'0'}]}], ecosystem_specific:{imports:openpgpImports}}]}},
    ],
  }
}
function check(change = () => {}, at = now) {
  const f = fixture()
  change(f)
  return evaluateSourceScan(f.messages.map(x=>JSON.stringify(x)).join('\n'),f.evidence,{now:at})
}
const withCrypto = version => f => {
  f.evidence.cryptoVersion = version
  f.messages[1].SBOM.modules[1].version = version
}
test('stream parser handles nested objects and escaped braces in strings', () => {
  const objects = [{progress:{message:'a } { " \\ b'}}, {osv:{x:[{y:1}]}}]
  assert.deepEqual(parseMessages(objects.map(x=>JSON.stringify(x,null,2)).join('\n')),objects)
})
for (const raw of ['', '  ', '{}junk', '{"config":', '[]', 'null', '{"a":1} {']) {
  test(`invalid/incomplete stream fails closed: ${JSON.stringify(raw)}`,()=>assert.throws(()=>parseMessages(raw)))
}
test('no findings is valid, OSV record alone is not a finding',()=>{
  const result=check()
  assert.equal(result.status,'pass')
  assert.equal(result.findings,0)
})
test('reviewed x/crypto fix pin passes clean, with no not-applicable items',()=>{
  const result=check(withCrypto(pin))
  assert.equal(result.status,'pass')
  assert.equal(result.findings,0)
  assert.deepEqual(result.crypto,{version:pin,pinReviewUntil:'2026-12-16T00:00:00.000Z'})
  assert.ok(!Object.hasOwn(result,'notApplicable'))
  assert.equal(result.goos,'darwin')
  assert.ok(result.limitations.includes('C/OS/Node/frontend not scanned'))
})
test('reviewed pin is accepted until 2026-12-16, then a tagged release is required',()=>{
  const fresh=version=>f=>{withCrypto(version)(f);f.messages[0].config.db_last_modified='2026-12-15T00:00:00Z'}
  assert.equal(check(fresh(pin),Date.parse('2026-12-15T23:59:59Z')).status,'pass')
  assert.throws(()=>check(fresh(pin),Date.parse('2026-12-16T00:00:00Z')),/pin expired/)
  assert.equal(check(fresh('v0.59.0'),Date.parse('2026-12-16T00:00:00Z')).status,'pass')
})
for(const version of [`${pin.slice(0,-1)}3`,'v0.58.1-0.20261010000000-2f1f834330b2','v0.59.0-rc.1',
  'v0.58.0+incompatible',`${pin}+incompatible`,'v0.58']) {
  test(`x/crypto ${version} is neither a tagged release nor the reviewed pin`,()=>assert.throws(()=>check(withCrypto(version))))
}
test('official repeated identical OSV is idempotent',()=>{
  assert.equal(check(f=>f.messages.push(structuredClone(f.messages[2]))).advisories,1)
})
test('conflicting repeated OSV fails closed',()=>assert.throws(()=>check(f=>{
  const conflict=structuredClone(f.messages[2])
  conflict.osv.summary='different metadata'
  f.messages.push(conflict)
})))
const invalid = {
  'missing config': f=>f.messages.shift(),
  'duplicate config': f=>f.messages.push(f.messages[0]),
  'duplicate SBOM': f=>f.messages.push(f.messages[1]),
  'missing SBOM': f=>f.messages.splice(1,1),
  'empty roots': f=>f.messages[1].SBOM.roots=[],
  'unknown protocol': f=>f.messages[0].config.protocol_version='v2',
  'wrong scanner': f=>f.messages[0].config.scanner_version='v1.0.0',
  'foreign DB': f=>f.messages[0].config.db='https://example.invalid',
  'stale DB': f=>f.messages[0].config.db_last_modified='2026-09-01T00:00:00Z',
  'future DB': f=>f.messages[0].config.db_last_modified='2026-10-18T00:00:00Z',
  'invalid DB date': f=>f.messages[0].config.db_last_modified='never',
  'wrong Go runtime': f=>f.messages[0].config.go_version='go1.26.5',
  'wrong SBOM runtime': f=>f.messages[1].SBOM.go_version='go1.24.13',
  'binary precision cannot masquerade as source': f=>f.messages[0].config.scan_mode='binary',
  'module precision cannot masquerade as symbol': f=>f.messages[0].config.scan_level='module',
  'unknown message': f=>f.messages.push({error:{message:'incomplete'}}),
  'ambiguous message': f=>f.messages.push({progress:{message:'x'},finding:{}}),
  'SBOM version mismatch': f=>f.messages[1].SBOM.modules[1].version='v0.1.0',
  'empty graph': f=>f.evidence.graphs.host=[],
  'missing target graph': f=>delete f.evidence.graphs['linux/amd64'],
  'missing main': f=>f.evidence.graphs.host=['golang.org/x/crypto/bcrypt'],
  'removed bcrypt': f=>f.evidence.graphs.host=['openreader/backend'],
  'CGO evidence absent': f=>f.evidence.cgoEnabled='0',
}
for(const [name,change] of Object.entries(invalid)) test(name,()=>assert.throws(()=>check(change)))
// The OpenPGP exception is retired: every finding blocks, whatever its precision, fix or metadata.
const trace=(frame={})=>[{module:'golang.org/x/crypto',version:'v0.57.0',...frame}]
const findings = {
  'formerly not-applicable pure module OpenPGP finding': {osv:'GO-2026-5932',trace:trace()},
  'module finding with the new fixed version (2026-11-03 live scan)': {osv:'GO-2026-5932',fixed_version:pin,trace:trace()},
  'unknown advisory': {osv:'GO-2099-9999',trace:trace()},
  'empty trace': {osv:'GO-2026-5932',trace:[]},
  'version mismatch': {osv:'GO-2026-5932',trace:trace({version:'v0.1.0'})},
  'missing version': {osv:'GO-2026-5932',trace:[{module:'golang.org/x/crypto'}]},
  'different module': {osv:'GO-2026-5932',trace:trace({module:'other/module'})},
  'package finding': {osv:'GO-2026-5932',trace:trace({package:'golang.org/x/crypto/openpgp'})},
  'symbol finding': {osv:'GO-2026-5932',trace:trace({package:'golang.org/x/crypto/openpgp',function:'ReadMessage'})},
}
for(const [name,finding] of Object.entries(findings)) {
  test(`finding blocks release: ${name}`,()=>assert.throws(()=>check(f=>f.messages.push({finding}))))
}
test('retired exception: the pre-fix v0.58.0 OpenPGP module finding is no longer not-applicable',()=>{
  assert.throws(()=>check(f=>{
    withCrypto('v0.58.0')(f)
    f.messages[2].osv.affected=[{package:{ecosystem:'Go',name:'golang.org/x/crypto'},
      ranges:[{type:'SEMVER',events:[{introduced:'0'}]}],ecosystem_specific:{imports:openpgpImports}}]
    f.messages.push({finding:{osv:'GO-2026-5932',trace:[{module:'golang.org/x/crypto',version:'v0.58.0'}]}})
  }),/block release/)
})
for(const graph of ['host','linux/amd64','linux/arm64']) {
  for(const path of ['golang.org/x/crypto/openpgp','golang.org/x/crypto/openpgp/packet']) {
    test(`${graph} imports ${path}: unmaintained OpenPGP blocks release without a finding`,()=>assert.throws(()=>check(f=>{
      f.evidence.graphs[graph]=[...f.evidence.graphs[graph],path]
    }),/OpenPGP imported/))
  }
}
test('owned scanner process must finish successfully',async()=>{
  assert.equal(await runTool(process.execPath,['-e','process.stdout.write("ok")']), 'ok')
  await assert.rejects(runTool(process.execPath,['-e','process.stdout.write("{}");process.exit(7)']))
})
test('owned tool optionally preserves successful stderr and real terminal status',async()=>{
  const result=await runTool(process.execPath,['-e','process.stdout.write("ok");process.stderr.write("owned warning")'],{captureOutput:true})
  assert.deepEqual(result,{stdout:'ok',stderr:'owned warning',exitCode:0,signal:null})
})
test('timeout kills and reaps owned scanner process group',async()=>{
  await assert.rejects(runTool(process.execPath,['-e',
    'require("child_process").spawn(process.execPath,["-e","setInterval(()=>{},1000)"],{stdio:"inherit"});setInterval(()=>{},1000)'],
  {timeout:150}),/timeout/)
})
test('caller interruption kills and reaps owned scanner before rejecting',async()=>{
  const pending=runTool(process.execPath,['-e','setInterval(()=>{},1000)'],{timeout:250})
  const timer=setTimeout(()=>process.emit('SIGTERM'),50)
  try {await assert.rejects(pending,/interrupted/)} finally {clearTimeout(timer)}
})
test('output limit fails rather than truncating to apparent clean result',async()=>{
  await assert.rejects(runTool(process.execPath,['-e','process.stdout.write("x".repeat(100000))'],{maxBytes:100}),/output/)
})
test('tool stream preserves UTF-8 split across chunks',async()=>{
  const result=await runTool(process.execPath,['-e',
    'const b=Buffer.from("正文");process.stdout.write(b.subarray(0,1));setTimeout(()=>process.stdout.write(b.subarray(1)),50)'])
  assert.equal(result,'正文')
})
test('release workflow triggers for gate changes and runs both checks before native build/publish',async()=>{
  const source=await readFile(new URL('../.github/workflows/docker-publish.yml',import.meta.url),'utf8')
  for(const path of ['scripts/check-go-advisories.mjs','scripts/check-go-advisories.test.mjs']) {
    assert.ok(source.slice(0,source.indexOf('workflow_dispatch:')).includes(`- ${path}`))
  }
  const tests=source.indexOf('run: node --test scripts/check-go-advisories.test.mjs')
  const scan=source.indexOf('run: node scripts/check-go-advisories.mjs')
  const build=source.indexOf('- name: Build native validation image')
  assert.ok(tests>0 && scan>tests && build>scan)
  assert.ok(!source.slice(tests,build).includes('continue-on-error'))
})

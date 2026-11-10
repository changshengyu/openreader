import assert from 'node:assert/strict'
import {createHash} from 'node:crypto'
import {readFile} from 'node:fs/promises'
import test from 'node:test'
import {gzipSync} from 'node:zlib'
import {evaluateOsInventory, flattenLayers, parseApkDatabase, parseArguments, readImageArchive, readTar} from './check-os-advisories.mjs'

// Real evidence: the published fc4692c base-layer databases, the 2026-10-13/10-09 v3.23 secdb records for the
// installed origins, native apk-tools 3.0.8 comparisons and the signed v3.23 main APKINDEX zlib records.
const data = name => readFile(new URL(`./testdata/os-apk/${name}`, import.meta.url), 'utf8')
const now = Date.parse('2026-10-16T12:00:00Z')
const baseline = JSON.parse(await readFile(new URL('./os-apk-baseline.json', import.meta.url), 'utf8'))
const published = {amd64:await data('fc4692c-linux-amd64-installed'), arm64:await data('fc4692c-linux-arm64-installed')}
const secdb = {main:JSON.parse(await data('secdb-v3.23-main-subset.json')), community:JSON.parse(await data('secdb-v3.23-community-subset.json'))}
const native = JSON.parse(await data('apk-version-native.json')).results
const zlibIndex = JSON.parse(await data('zlib-1.3.2-r1-apkindex.json'))
// Signed v3.23 main APKINDEX records plus the real installed blocks after the exact build-time musl install,
// the 2026-10-21 v3.23 main secdb records for the installed origins and the extra native comparisons it plans.
const muslIndex = JSON.parse(await data('musl-1.2.5-r24-apkindex.json'))
const secdbMusl = JSON.parse(await data('secdb-v3.23-main-subset-2026-10-21.json'))
const nativeMusl = JSON.parse(await data('apk-version-native-2026-10-21-musl.json')).results
// Signed v3.23 main APKINDEX records plus the real installed blocks after the exact build-time zlib 1.4-r0 install,
// the 2026-11-10 main and 2026-11-07 community secdb records for the installed origins and the native comparisons they add.
const zlib14Index = JSON.parse(await data('zlib-1.4-r0-apkindex.json'))
const secdbZlib = {main:JSON.parse(await data('secdb-v3.23-main-subset-2026-11-10.json')),
  community:JSON.parse(await data('secdb-v3.23-community-subset-2026-11-07.json'))}
const nativeZlib = JSON.parse(await data('apk-version-native-2026-11-10-zlib.json')).results
// Signed v3.23 main APKINDEX records plus the real installed blocks after the exact build-time ca-certificates-bundle
// 20261020-r0 install from the pristine base bundle (NSS 3.131.1 drops three eMudhra emSign roots), with the bundle diff.
const caIndex = JSON.parse(await data('ca-certificates-bundle-20261020-r0-apkindex.json'))
const apkSum = value => `Q1${createHash('sha1').update(value).digest('base64')}`
const apkArch = arch => arch === 'amd64' ? 'x86_64' : 'aarch64'
// usr/lib/os-release is owned by alpine-release, so the fixture must be the exact published bytes.
const osRelease = 'NAME="Alpine Linux"\nID=alpine\nVERSION_ID=3.23.6\nPRETTY_NAME="Alpine Linux v3.23"\n'
  + 'HOME_URL="https://alpinelinux.org/"\nBUG_REPORT_URL="https://gitlab.alpinelinux.org/alpine/aports/-/issues"\n'

// zlib is false (the published 1.3.2-r0), '1.3.2-r1' (the 2026-10-16/10-22 releases) or '1.4-r0' (the 2026-11-10 release);
// ca false keeps the base's own ca-certificates-bundle 20260909-r0 that every release up to 74cf0d0 shipped.
function fixedDatabase(arch, {zlib = '1.4-r0', musl = true, ca = true} = {}) {
  const blocks = published[arch].trim().split('\n\n')
  const replace = (name, block) => {
    blocks[blocks.findIndex(item => item.includes(`\nP:${name}\n`))] = block
  }
  // The signed index record carries the package identity; file hashes come from the package itself.
  if (zlib === '1.3.2-r1') replace('zlib', `${zlibIndex[apkArch(arch)].record}\nF:usr\nF:usr/lib\nR:libz.so.1\na:0:0:777\nZ:${apkSum('libz.so.1.3.2')}`
    + `\nR:libz.so.1.3.2\na:0:0:755\nZ:${apkSum(`fixture zlib 1.3.2-r1 ${arch}`)}`)
  if (zlib === '1.4-r0') replace('zlib', zlib14Index[apkArch(arch)].installed)
  if (musl) for (const name of ['musl', 'musl-utils']) replace(name, muslIndex[apkArch(arch)].installed[name])
  if (ca) replace('ca-certificates-bundle', caIndex[apkArch(arch)].installed)
  return `${blocks.join('\n\n')}\n\n`
}
const candidateDatabase = arch => fixedDatabase(arch)
const releasedDatabase = arch => fixedDatabase(arch, {ca:false})
const previousCandidateDatabase = arch => fixedDatabase(arch, {zlib:'1.3.2-r1', ca:false})
const zlibFixedDatabase = arch => fixedDatabase(arch, {zlib:'1.3.2-r1', musl:false, ca:false})
const muslFixedDatabase = arch => fixedDatabase(arch, {zlib:false, ca:false})
function filesFor(database, world) {
  const files = new Map()
  let folder = null, path = null
  for (const line of database.split('\n')) {
    if (line.startsWith('F:')) folder = line.slice(2)
    else if (line.startsWith('R:')) path = `${folder}/${line.slice(2)}`
    else if (line.startsWith('Z:')) files.set(path, {type:'file', checksum:line.slice(2)})
  }
  files.set('etc/shadow', {type:'file', checksum:baseline.modifiedFiles['etc/shadow'].checksum})
  const texts = {'lib/apk/db/installed':database, 'usr/lib/os-release':osRelease,
    'etc/apk/world':`${world.join('\n')}\n`, 'etc/apk/repositories':`${baseline.repositories.join('\n')}\n`}
  for (const [name, text] of Object.entries(texts)) files.set(name, {type:'file', checksum:apkSum(text), content:Buffer.from(text)})
  return files
}
function inputs(change = () => {}, {database = candidateDatabase, world = baseline.world, policy = baseline} = {}) {
  const image = platform => {
    const arch = platform.split('/')[1]
    return {platform, files:filesFor(database(arch), world), config:{os:'linux', architecture:arch,
      ...(arch === 'arm64' ? {variant:'v8'} : {}), config:{Cmd:['/app/openreader'], Entrypoint:null}}}
  }
  // The collector runs exactly the planned native comparisons for the installed versions.
  const installed = new Set([...database('amd64').matchAll(/^V:(.*)$/gm)].map(match => match[1]))
  const value = {baseline:structuredClone(policy), images:[image('linux/amd64'), image('linux/arm64')],
    secdb:{main:{document:structuredClone(secdb.main), lastModified:'Tue, 13 Oct 2026 20:35:17 GMT'},
      community:{document:structuredClone(secdb.community), lastModified:'Fri, 09 Oct 2026 18:09:41 GMT'}},
    comparisons:new Map(Object.entries(native).filter(([pair]) => installed.has(pair.split('|')[0]))), now}
  change(value)
  return value
}
const check = (change, options) => evaluateOsInventory(inputs(change, options))
const setText = (image, path, text) => image.files.set(path, {type:'file', checksum:apkSum(text), content:Buffer.from(text)})
// etc/apk/world of the pinned official alpine:3.23.6 base before any build-time install.
const officialBaseWorld = ['alpine-baselayout','alpine-keys','alpine-release','apk-tools','busybox','musl-utils']
const publishedPolicy = (names, from = baseline) => {
  // The given policy with the named packages back at their exact published identities and world entries.
  const policy = structuredClone(from)
  for (const [platform, arch] of [['linux/amd64','amd64'],['linux/arm64','arm64']]) {
    for (const name of names) {
      const block = published[arch].split('\n\n').find(item => item.includes(`\nP:${name}\n`))
      policy.platforms[platform].packages[name] = {version:/^V:(.*)$/m.exec(block)[1], origin:/^o:(.*)$/m.exec(block)[1],
        checksum:/^C:(.*)$/m.exec(block)[1]}
    }
  }
  policy.world = policy.world.flatMap(item => {
    const name = item.split('=')[0]
    if (!names.includes(name)) return [item]
    return officialBaseWorld.includes(name) ? [name] : []
  })
  return policy
}
// The 2026-11-10 release 74cf0d0: zlib 1.4-r0 and musl 1.2.5-r24 with the base's own ca-certificates-bundle 20260909-r0.
const releasePolicy = publishedPolicy(['ca-certificates-bundle'])
// The 2026-10-22 release: both earlier build-time fixes, zlib at the signed 1.3.2-r1.
const previousPolicy = (() => {
  const policy = structuredClone(releasePolicy)
  for (const [platform, arch] of [['linux/amd64','amd64'],['linux/arm64','arm64']]) {
    policy.platforms[platform].packages.zlib = {version:'1.3.2-r1', origin:'zlib',
      checksum:/^C:(.*)$/m.exec(zlibIndex[apkArch(arch)].record)[1]}
  }
  policy.world = policy.world.map(item => item.startsWith('zlib=') ? 'zlib=1.3.2-r1' : item)
  return policy
})()
// The pinned official base plus only the zlib 1.3.2-r1 build-time fix, and the pinned official base before any build-time fix.
const zlibOnly = publishedPolicy(['musl', 'musl-utils'], previousPolicy)
const baseOnly = publishedPolicy(['musl', 'musl-utils', 'zlib', 'ca-certificates-bundle'])
const secdbOn20261021 = value => {
  value.secdb.main = {document:structuredClone(secdbMusl), lastModified:'Wed, 21 Oct 2026 18:11:19 GMT'}
  value.now = Date.parse('2026-10-22T12:00:00Z')
}
// The secdb the 2026-11-10 release run read: since 10-21 only zlib 1.4-r0 (CVE-2026-4738) is new for the installed origins.
// Its musl 1.2.5-r24 fix and the zlib fix each add one planned native comparison for the installed versions.
const secdbOn20261110 = (value, zlibVersion) => {
  value.secdb.main = {document:structuredClone(secdbZlib.main), lastModified:'Tue, 10 Nov 2026 07:25:11 GMT'}
  value.secdb.community = {document:structuredClone(secdbZlib.community), lastModified:'Sat, 07 Nov 2026 07:30:27 GMT'}
  value.now = Date.parse('2026-11-10T12:00:00Z')
  value.comparisons.set('1.2.5-r24|1.2.5-r24', nativeMusl['1.2.5-r24|1.2.5-r24'])
  value.comparisons.set(`${zlibVersion}|1.4-r0`, nativeZlib[`${zlibVersion}|1.4-r0`])
}

test('pre-fix policies keep the exact published identities and drop only the build-time pins', () => {
  assert.deepEqual([...baseOnly.world].sort(), officialBaseWorld)
  assert.deepEqual([...zlibOnly.world].sort(), [...officialBaseWorld, 'zlib=1.3.2-r1'])
  assert.deepEqual([...previousPolicy.world].sort(), [...officialBaseWorld.filter(name => name !== 'musl-utils'),
    'musl-utils=1.2.5-r24', 'musl=1.2.5-r24', 'zlib=1.3.2-r1'].sort())
  assert.ok(baseline.world.includes('ca-certificates-bundle=20261020-r0'))
  assert.deepEqual([...releasePolicy.world].sort(), baseline.world.filter(item => !item.startsWith('ca-certificates-bundle=')).sort())
  for (const platform of ['linux/amd64', 'linux/arm64']) {
    assert.equal(baseline.platforms[platform].packages['ca-certificates-bundle'].version, '20261020-r0')
    for (const policy of [baseOnly, zlibOnly, previousPolicy, releasePolicy])
      assert.equal(policy.platforms[platform].packages['ca-certificates-bundle'].version, '20260909-r0')
    assert.equal(releasePolicy.platforms[platform].packages.zlib.version, '1.4-r0')
    assert.equal(baseOnly.platforms[platform].packages.zlib.version, '1.3.2-r0')
    assert.equal(zlibOnly.platforms[platform].packages.zlib.version, '1.3.2-r1')
    assert.equal(previousPolicy.platforms[platform].packages.zlib.version, '1.3.2-r1')
    assert.equal(previousPolicy.platforms[platform].packages.musl.version, '1.2.5-r24')
    for (const policy of [baseOnly, zlibOnly]) {
      assert.equal(policy.platforms[platform].packages.musl.version, '1.2.5-r23')
      assert.equal(policy.platforms[platform].packages['musl-utils'].version, '1.2.5-r23')
    }
  }
})
test('published fc4692c and earlier release inventories are rejected by the reviewed identity pin on both platforms', () => {
  // Every release up to 74cf0d0 ships the base's ca-certificates-bundle 20260909-r0, the first pinned package in database order.
  for (const [database, world] of [[arch => published[arch], baseOnly.world], [zlibFixedDatabase, zlibOnly.world],
    [muslFixedDatabase, baseOnly.world], [previousCandidateDatabase, previousPolicy.world], [releasedDatabase, releasePolicy.world]]) {
    assert.throws(() => check(() => {}, {database, world}), /ca-certificates-bundle 20260909-r0 .* expected 20261020-r0/s)
  }
  // With the CA bundle fixed, each older musl or zlib identity is still rejected by its own pin.
  assert.throws(() => check(() => {}, {database:arch => fixedDatabase(arch, {zlib:false, musl:false})}),
    /musl 1\.2\.5-r23 .* expected 1\.2\.5-r24/s)
  assert.throws(() => check(() => {}, {database:arch => fixedDatabase(arch, {zlib:'1.3.2-r1', musl:false})}),
    /musl 1\.2\.5-r23 .* expected 1\.2\.5-r24/s)
  assert.throws(() => check(() => {}, {database:arch => fixedDatabase(arch, {zlib:false})}),
    /zlib.*1\.3\.2-r0.*1\.4-r0/s)
  assert.throws(() => check(() => {}, {database:arch => fixedDatabase(arch, {zlib:'1.3.2-r1'})}),
    /zlib.*1\.3\.2-r1.*1\.4-r0/s)
})
test('pinned base with only the zlib fix fails on the exact pending musl secfix of the 2026-10-21 secdb', () => {
  assert.throws(() => check(value => {
    secdbOn20261021(value)
    value.comparisons.set('1.2.5-r23|1.2.5-r24', nativeMusl['1.2.5-r23|1.2.5-r24'])
  }, {database:zlibFixedDatabase, world:zlibOnly.world, policy:zlibOnly}),
  error => /pending Alpine secfix/.test(error.message) && ['linux/amd64 musl', 'linux/amd64 musl-utils', 'linux/arm64 musl',
    'linux/arm64 musl-utils'].every(entry => error.message.includes(`${entry} 1.2.5-r23 < 1.2.5-r24 CVE-2026-107891`)))
})
test('candidate with exact signed musl 1.2.5-r24 passes the 2026-10-21 secdb', () => {
  const summary = check(value => {
    secdbOn20261021(value)
    value.comparisons.set('1.2.5-r24|1.2.5-r24', nativeMusl['1.2.5-r24|1.2.5-r24'])
  })
  assert.equal(summary.status, 'pass-with-unknown-origins')
  assert.deepEqual(summary.secdb.map(db => [db.name, db.lastModified]), [['main','2026-10-21T18:11:19.000Z'],['community','2026-10-09T18:09:41.000Z']])
  assert.equal(summary.comparisons, 73)
})
test('previous release with zlib 1.3.2-r1 fails on the exact pending zlib secfix of the 2026-11-10 secdb', () => {
  // The same rejection as trusted run 42775373137, from the candidate it built.
  assert.throws(() => check(value => secdbOn20261110(value, '1.3.2-r1'),
    {database:previousCandidateDatabase, world:previousPolicy.world, policy:previousPolicy}),
  error => /pending Alpine secfix/.test(error.message) && ['linux/amd64', 'linux/arm64']
    .every(platform => error.message.includes(`${platform} zlib 1.3.2-r1 < 1.4-r0 CVE-2026-4738`))
    && !/musl|busybox|apk-tools|openssl|libcrypto|libssl/.test(error.message))
})
test('candidate with exact signed zlib 1.4-r0 passes the 2026-11-10 secdb', () => {
  const summary = check(value => secdbOn20261110(value, '1.4-r0'))
  assert.equal(summary.status, 'pass-with-unknown-origins')
  assert.deepEqual(summary.secdb.map(db => [db.name, db.lastModified]), [['main','2026-11-10T07:25:11.000Z'],['community','2026-11-07T07:30:27.000Z']])
  assert.deepEqual(summary.platforms.map(p => [p.platform, p.packages, p.files]), [['linux/amd64',16,110],['linux/arm64',16,109]])
  assert.equal(summary.comparisons, 74)
  assert.deepEqual(summary.buildTimeFixes.zlib, {from:'1.3.2-r0', to:'1.4-r0', ids:['CVE-2026-85091', 'CVE-2026-4738'],
    aportsCommit:'0436c6d655a04cbfd7987202a970f5aa7870a901', index:'v3.23.8-20-g8b367fdee54'})
})
test('candidate with exact signed ca-certificates-bundle 20261020-r0 passes the 2026-11-10 secdb under the renewed review', () => {
  const summary = check(value => secdbOn20261110(value, '1.4-r0'))
  assert.equal(summary.status, 'pass-with-unknown-origins')
  assert.deepEqual(summary.platforms.map(p => [p.platform, p.packages, p.files]), [['linux/amd64',16,110],['linux/arm64',16,109]])
  assert.equal(summary.comparisons, 74)
  assert.deepEqual(summary.buildTimeFixes['ca-certificates'], {from:'20260909-r0', to:'20261020-r0', ids:['CVE-2026-22063', 'CVE-2026-114406'],
    aportsCommit:'ac857c298f6fd212498ce76bb87836d940cdba2d', index:'v3.23.8-20-g8b367fdee54'})
  assert.deepEqual(summary.unknownOrigins, ['alpine-base','alpine-baselayout','alpine-keys','ca-certificates','pax-utils'])
  assert.equal(summary.reviewUntil, '2026-12-16T00:00:00.000Z')
  // The pinned identity is the signed index record; the bundle file is the package's own 118-root copy.
  for (const [platform, arch] of [['linux/amd64','x86_64'],['linux/arm64','aarch64']]) {
    assert.equal(baseline.platforms[platform].packages['ca-certificates-bundle'].checksum, /^C:(.*)$/m.exec(caIndex[arch].record)[1])
    assert.ok(caIndex[arch].installed.includes(`\nR:ca-certificates.crt\nZ:${caIndex.bundle.after.checksum}\n`))
  }
  assert.deepEqual(caIndex.bundle.removed.map(root => /^CN=([^,]+)/.exec(root.subject)[1]),
    ['emSign ECC Root CA - C3', 'emSign ECC Root CA - G3', 'emSign Root CA - C1'])
  assert.deepEqual(caIndex.bundle.added, [])
})
test('a CA bundle kept beside .apk-new fails closed on the APK-owned bundle file', () => {
  // apk keeps a modified protected file and writes the package copy as .apk-new (seen locally when the container runtime
  // injects its own root into the bundle): the database then records the new bundle while the old bytes stay in place.
  assert.throws(() => check(value => {
    for (const image of value.images) {
      image.files.set('etc/ssl/certs/ca-certificates.crt', {type:'file', checksum:caIndex.bundle.before.checksum})
      image.files.set('etc/ssl/certs/ca-certificates.crt.apk-new', {type:'file', checksum:caIndex.bundle.after.checksum})
    }
  }), /APK file changed after install: etc\/ssl\/certs\/ca-certificates\.crt$/)
})
test('pinned base without the build-time fix fails on the exact pending zlib secfix, not a lexical compare', () => {
  assert.throws(() => check(() => {}, {database:arch => published[arch], world:baseOnly.world, policy:baseOnly}),
    error => /pending Alpine secfix/.test(error.message) && /linux\/amd64 zlib 1\.3\.2-r0 < 1\.3\.2-r1 CVE-2026-85091/.test(error.message)
      && /linux\/arm64 zlib 1\.3\.2-r0 < 1\.3\.2-r1 CVE-2026-85091/.test(error.message))
})
test('candidate with exact signed zlib 1.4-r0 and musl 1.2.5-r24 passes and reports unknown origins, never clean', () => {
  const summary = check()
  assert.equal(summary.status, 'pass-with-unknown-origins')
  assert.deepEqual(summary.unknownOrigins, ['alpine-base','alpine-baselayout','alpine-keys','ca-certificates','pax-utils'])
  assert.deepEqual(summary.platforms.map(p => [p.platform, p.packages, p.files]), [['linux/amd64',16,110],['linux/arm64',16,109]])
  assert.deepEqual(summary.modifiedFiles, ['etc/shadow'])
  assert.equal(summary.comparisons, 72)
  assert.deepEqual(summary.secdb.map(db => [db.name, db.lastModified]), [['main','2026-10-13T20:35:17.000Z'],['community','2026-10-09T18:09:41.000Z']])
  assert.ok(summary.limitations.some(item => /not clean/.test(item)) && summary.limitations.some(item => /not a whole-image/.test(item)))
})
test('APK database parser rejects empty, truncated, unknown-field, orphan and duplicate records', () => {
  const valid = candidateDatabase('amd64')
  assert.equal(parseApkDatabase(valid, 'x86_64').size, 16)
  const broken = [
    '', '\n', valid.slice(0, -2).replace(/\n[^\n]*$/, ''), valid.replace(/^V:1\.4-r0\n/m, ''),
    valid.replace(/^o:zlib\n/m, ''), valid.replace(/^C:Q1w8ww[^\n]*\n/m, ''), valid.replace(/^L:Zlib\n/m, 'L:Zlib\nY:new\n'),
    valid.replace(/(P:zlib[\s\S]*?)F:usr\nF:usr\/lib\n/, '$1'), valid.replace(/\nZ:Q1[^\n]*\n\n/, '\n\n'), valid.replace(/^T:A compression/m, 'T:A compression\nT:twice'),
    `${valid}${valid.split('\n\n')[0]}\n\n`, valid.replace('V:1.4-r0', 'V:1.4 r0'), valid.replace('C:Q1w8ww', 'C:Q2w8ww'),
    valid.replace(/^R:libz\.so\.1$/m, 'R:../libz.so.1'),
  ]
  // Every mutation must change the text, or the assertion below would pass vacuously.
  for (const text of broken) assert.notEqual(text, valid)
  for (const text of broken) assert.throws(() => parseApkDatabase(text, 'x86_64'), undefined, JSON.stringify(text.slice(-60)))
})
test('wrong package architecture fails closed', () => {
  assert.throws(() => parseApkDatabase(candidateDatabase('arm64'), 'x86_64'), /architecture/)
  assert.throws(() => check(value => {
    const text = candidateDatabase('arm64').replace('A:aarch64', 'A:x86_64')
    setText(value.images[1], 'lib/apk/db/installed', text)
  }), /architecture/)
})
test('image platform, variant, command and entrypoint must match the release image', () => {
  for (const change of [
    value => {value.images[1].config.architecture = 'amd64'},
    value => {value.images[1].config.variant = 'v7'},
    value => {value.images[0].config.os = 'windows'},
    value => {value.images[0].config.config.Cmd = ['/bin/sh','-c','apk upgrade && /app/openreader']},
    value => {value.images[0].config.config.Entrypoint = ['/sbin/apk']},
    value => {value.images.pop()},
    value => {value.images[1].platform = 'linux/amd64'},
    value => {value.images.push(value.images[0])},
  ]) assert.throws(() => check(change))
})
test('os-release, repositories and world must match the reviewed baseline exactly', () => {
  for (const change of [
    value => setText(value.images[0], 'usr/lib/os-release', osRelease.replace('3.23.6', '3.23.7')),
    value => setText(value.images[0], 'usr/lib/os-release', osRelease.replace('ID=alpine', 'ID=debian')),
    value => value.images[0].files.delete('usr/lib/os-release'),
    value => setText(value.images[1], 'etc/apk/repositories', `${baseline.repositories.join('\n')}\nhttps://example.invalid/alpine/edge/testing\n`),
    value => setText(value.images[1], 'etc/apk/repositories', baseline.repositories.join('\n').replaceAll('https://', 'http://')),
    value => setText(value.images[0], 'etc/apk/world', `${baseline.world.join('\n').replace('zlib=1.4-r0', 'zlib')}\n`),
    value => setText(value.images[0], 'etc/apk/world', `${baseline.world.join('\n').replace('zlib=1.4-r0', 'zlib=1.3.2-r1')}\n`),
    value => setText(value.images[0], 'etc/apk/world', `${baseline.world.join('\n')}\nopenssl\n`),
  ]) assert.throws(() => check(change))
})
test('package identity drift, extra and missing packages fail closed', () => {
  const amd64 = candidateDatabase('amd64')
  for (const text of [
    amd64.replace('C:Q1w8ww/Ov6R+0J7O4tgeMeDsf/vp0=', `C:${apkSum('locally rebuilt zlib')}`),
    amd64.replace('V:1.4-r0', 'V:1.4-r1'),
    `${amd64}C:${apkSum('extra')}\nP:extra\nV:1.0-r0\nA:x86_64\no:extra\n\n`,
    amd64.split('\n\n').filter(block => !block.includes('\nP:scanelf\n')).join('\n\n'),
  ]) {
    assert.notEqual(text, amd64)
    assert.throws(() => check(value => setText(value.images[0], 'lib/apk/db/installed', text)), /identity|package/)
  }
})
test('overlay changes to APK-owned files fail closed; only the pinned official modification is allowed', () => {
  assert.throws(() => check(value => value.images[0].files.set('usr/lib/libz.so.1.4', {type:'file', checksum:apkSum('old zlib')})), /usr\/lib\/libz\.so\.1\.4/)
  assert.throws(() => check(value => value.images[1].files.delete('usr/lib/libz.so.1')), /usr\/lib\/libz\.so\.1/)
  assert.throws(() => check(value => value.images[0].files.set('etc/shadow', {type:'file', checksum:apkSum('changed again')})), /etc\/shadow/)
  assert.throws(() => check(value => value.images[0].files.set('etc/ssl/certs/ca-certificates.crt', {type:'file', checksum:apkSum('builder bundle')})), /ca-certificates\.crt/)
  assert.throws(() => check(value => {value.baseline.modifiedFiles['etc/shadow'].packageChecksum = apkSum('other')}), /etc\/shadow/)
})
test('cross-platform package sets must match', () => {
  assert.throws(() => check(value => {
    value.baseline.platforms['linux/arm64'].packages.busybox.version = '1.37.0-r31'
    setText(value.images[1], 'lib/apk/db/installed', candidateDatabase('arm64').replace('P:busybox\nV:1.37.0-r30', 'P:busybox\nV:1.37.0-r31'))
  }), /platform/)
})
test('secdb schema, freshness and identifiers fail closed', () => {
  for (const change of [
    value => {value.secdb.main.document.distroversion = 'v3.24'},
    value => {value.secdb.main.document.reponame = 'community'},
    value => {value.secdb.community.document.urlprefix = 'http://dl-cdn.alpinelinux.org/alpine'},
    value => {value.secdb.main.document.archs = ['x86_64']},
    value => {value.secdb.main.document.packages = []},
    value => {delete value.secdb.community},
    value => {value.secdb.main.document.packages[0].pkg.secfixes = []},
    value => {value.secdb.main.document.packages[0].pkg.secfixes['1.0-r0'] = 'CVE-1'},
    value => {value.secdb.main.document.packages[0].pkg.secfixes['1.0-r0'] = ['']},
    value => {value.secdb.main.document.packages[0].pkg.secfixes['1.0 r0'] = ['CVE-1']},
    value => {value.secdb.main.document.packages[0].pkg.name = ''},
    value => {value.secdb.main.lastModified = 'Mon, 14 Sep 2026 00:00:00 GMT'},
    value => {value.secdb.community.lastModified = 'Sat, 17 Oct 2026 13:00:00 GMT'},
    value => {value.secdb.main.lastModified = null},
  ]) assert.throws(() => check(change))
})
test('every planned native comparison must be present and valid', () => {
  assert.throws(() => check(value => value.comparisons.delete('1.4-r0|1.3.2-r1')), /comparison/)
  assert.throws(() => check(value => value.comparisons.set('1.4-r0|1.3.2-r1', '<=')), /comparison/)
  assert.throws(() => check(value => value.comparisons.set('9.9-r9|0', '>')), /comparison/)
  // A native "<" for an installed version is a real pending result, never coerced to clean.
  assert.throws(() => check(value => value.comparisons.set('1.4-r0|1.3.2-r1', '<')), /pending Alpine secfix/)
})
test('unlisted origins require explicit unexpired review and become stale once secdb lists them', () => {
  assert.throws(() => check(value => {delete value.baseline.unlistedOrigins['pax-utils']}), /pax-utils/)
  assert.throws(() => check(value => {value.now = Date.parse('2026-12-16T00:00:00Z')
    value.secdb.main.lastModified = value.secdb.community.lastModified = 'Tue, 15 Dec 2026 00:00:00 GMT'}), /review/)
  assert.throws(() => check(value => value.secdb.main.document.packages.push({pkg:{name:'pax-utils', secfixes:{'1.3.8-r2':['CVE-2026-0001']}}})), /pax-utils/)
})
test('the 2026-11-10 unlisted-origin review stays valid through 2026-12-15 and expires at 2026-12-16T00:00:00Z', () => {
  const at = (now, lastModified) => value => {value.now = Date.parse(now)
    value.secdb.main.lastModified = value.secdb.community.lastModified = lastModified}
  const summary = check(at('2026-12-15T23:59:59Z', 'Mon, 14 Dec 2026 00:00:00 GMT'))
  assert.equal(summary.status, 'pass-with-unknown-origins')
  assert.equal(summary.reviewUntil, '2026-12-16T00:00:00.000Z')
  assert.throws(() => check(at('2026-12-16T00:00:00Z', 'Tue, 15 Dec 2026 00:00:00 GMT')), /unlisted origin review expired: alpine-base/)
  // Each review records the 2026-11-10 evidence instead of the 2026-10-13 snapshot it replaces.
  assert.deepEqual(Object.keys(baseline.unlistedOrigins).sort(), ['alpine-base','alpine-baselayout','alpine-keys','ca-certificates','pax-utils'])
  for (const [origin, reason] of Object.entries(baseline.unlistedOrigins)) assert.match(reason, /2026-11-10/, origin)
})

function header(name, size, type = '0', link = '') {
  const block = Buffer.alloc(512)
  block.write(name, 0, 100); block.write('0000644\0', 100); block.write('0000000\0', 108); block.write('0000000\0', 116)
  block.write(`${size.toString(8).padStart(11, '0')}\0`, 124); block.write('00000000000\0', 136); block.write('        ', 148)
  block.write(type, 156); block.write(link, 157, 100); block.write('ustar\0', 257); block.write('00', 263)
  let sum = 0
  for (const byte of block) sum += byte
  block.write(`${sum.toString(8).padStart(6, '0')}\0 `, 148)
  return block
}
const pad = bytes => Buffer.concat([bytes, Buffer.alloc((512 - bytes.length % 512) % 512)])
const tar = entries => Buffer.concat([...entries.flatMap(({name, data = '', type = '0', link = ''}) => {
  const body = Buffer.isBuffer(data) ? data : Buffer.from(data)
  return [header(name, body.length, type, link), pad(body)]
}), Buffer.alloc(1024)])
function paxRecord(key, value) {
  const body = ` ${key}=${value}\n`
  let length = body.length + 1
  while (String(length).length + body.length !== length) length = String(length).length + body.length
  return `${length}${body}`
}
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex')

test('tar reader verifies checksums, end marker, pax/GNU names and rejects truncation and unsafe paths', () => {
  const longName = `usr/lib/${'n'.repeat(120)}`
  const archive = tar([{name:'./usr/', type:'5'}, {name:'usr/lib/libz.so.1.3.2', data:'zlib'},
    {name:'PaxHeaders/x', type:'x', data:paxRecord('path', 'usr/lib/pax-name')}, {name:'short', data:'pax'},
    {name:'././@LongLink', type:'L', data:`${longName}\0`}, {name:'trunc', data:'gnu'},
    {name:'usr/lib/libz.so.1', type:'2', link:'libz.so.1.3.2'}])
  const entries = readTar(archive)
  assert.deepEqual(entries.map(e => [e.path, e.type]), [['usr','5'],['usr/lib/libz.so.1.3.2','0'],['usr/lib/pax-name','0'],[longName,'0'],['usr/lib/libz.so.1','2']])
  assert.equal(entries[4].link, 'libz.so.1.3.2')
  const corrupt = Buffer.from(archive); corrupt[1024] ^= 1
  const badHeader = Buffer.from(archive); badHeader[0] ^= 1
  for (const bytes of [archive.subarray(0, archive.length - 1024), archive.subarray(0, 1024 + 512), badHeader,
    tar([{name:'../etc/passwd', data:'x'}]), tar([{name:'/etc/passwd', data:'x'}]), tar([{name:'dev/x', type:'S'}]),
    Buffer.concat([archive, Buffer.from('trailing data')]), Buffer.alloc(0)]) assert.throws(() => readTar(bytes))
  assert.doesNotThrow(() => readTar(corrupt), 'file data is covered by layer digests, not tar header checksums')
})
test('layer flattening applies whiteouts, opaque directories and hardlinks like the image runtime', () => {
  const lower = readTar(tar([{name:'usr/lib/a', data:'one'}, {name:'etc/x', data:'x'}, {name:'usr/lib/l', type:'2', link:'a'}]))
  const upper = readTar(tar([{name:'usr/lib/.wh.a'}, {name:'etc/.wh..wh..opq'}, {name:'etc/y', data:'y'},
    {name:'usr/h', type:'1', link:'etc/y'}, {name:'usr/lib/l2', type:'2', link:'../h'}]))
  const files = flattenLayers([lower, upper])
  assert.ok(!files.has('usr/lib/a') && !files.has('etc/x'))
  assert.equal(files.get('etc/y').checksum, apkSum('y'))
  assert.equal(files.get('usr/h').checksum, apkSum('y'))
  assert.equal(files.get('usr/lib/l').checksum, apkSum('a'))
  assert.equal(files.get('usr/lib/l2').type, 'symlink')
  assert.throws(() => flattenLayers([readTar(tar([{name:'usr/h', type:'1', link:'missing'}]))]), /hardlink/)
})
test('docker save archive requires one manifest and layers matching config diff_ids', () => {
  const layer = tar([{name:'lib/apk/db/installed', data:'db'}, {name:'usr/lib/os-release', data:osRelease}])
  const plain = tar([{name:'app/openreader', data:'binary'}])
  const compressed = gzipSync(plain)
  const blob = bytes => `blobs/sha256/${sha256(bytes)}`
  const archive = ({layers = [layer, compressed], diffIds = [layer, plain].map(b => `sha256:${sha256(b)}`), manifests = 1,
    rootfs = 'layers', configPath} = {}) => {
    const config = Buffer.from(JSON.stringify({os:'linux', architecture:'arm64', variant:'v8',
      config:{Cmd:['/app/openreader']}, rootfs:{type:rootfs, diff_ids:diffIds}}))
    const manifest = Array.from({length:manifests}, () => ({Config:configPath ?? blob(config),
      RepoTags:['openreader-ci-arm64:test'], Layers:layers.map(blob)}))
    return tar([{name:'manifest.json', data:JSON.stringify(manifest)}, {name:configPath ?? blob(config), data:config},
      ...layers.map(bytes => ({name:blob(bytes), data:bytes}))])
  }
  const image = readImageArchive(archive())
  assert.equal(image.config.architecture, 'arm64')
  assert.equal(image.files.get('usr/lib/os-release').content.toString(), osRelease)
  assert.equal(image.files.get('app/openreader').checksum, apkSum('binary'))
  for (const options of [{diffIds:[layer, compressed].map(b => `sha256:${sha256(b)}`)}, {manifests:2}, {manifests:0},
    {diffIds:[`sha256:${sha256(layer)}`]}, {layers:[layer, Buffer.concat([Buffer.from([0x28,0xb5,0x2f,0xfd]), Buffer.alloc(508)])]},
    {rootfs:'other'}, {configPath:`blobs/sha256/${'0'.repeat(64)}`}]) assert.throws(() => readImageArchive(archive(options)))
})
test('collector arguments accept only the two release image references', () => {
  assert.deepEqual(parseArguments(['openreader-ci:abc123', 'openreader-ci-arm64:abc123']),
    {'linux/amd64':'openreader-ci:abc123', 'linux/arm64':'openreader-ci-arm64:abc123'})
  for (const argv of [[], ['openreader-ci:a'], ['openreader-ci:a','openreader-ci-arm64:a','--skip'], ['--allow-pending','openreader-ci-arm64:a'],
    ['openreader-ci:a','openreader-ci-arm64:b'], ['openreader-ci-arm64:a','openreader-ci:a'],
    ['ghcr.io/x/y:latest','openreader-ci-arm64:a'], ['openreader-ci:a','openreader-ci-arm64:a;rm'], ['openreader-ci@sha256:00','openreader-ci-arm64:a']])
    assert.throws(() => parseArguments(argv))
})
test('release workflow builds both validation images and runs the OS gate before publish', async () => {
  const source = await readFile(new URL('../.github/workflows/docker-publish.yml', import.meta.url), 'utf8')
  const triggers = source.slice(0, source.indexOf('workflow_dispatch:'))
  for (const path of ['scripts/check-os-advisories.mjs', 'scripts/check-os-advisories.test.mjs', 'scripts/os-apk-baseline.json', 'scripts/testdata/os-apk/**'])
    assert.ok(triggers.includes(`- ${path}`), path)
  const at = text => {const index = source.indexOf(text); assert.ok(index > 0, text); return index}
  const tests = at('run: node --test scripts/check-os-advisories.test.mjs')
  const amd64 = at('- name: Build native validation image'), arm64 = at('- name: Build arm64 validation image')
  const gate = at('run: node scripts/check-os-advisories.mjs openreader-ci:${{ github.sha }} openreader-ci-arm64:${{ github.sha }}')
  const login = at('- name: Log in to GHCR'), publish = at('- name: Build and publish multi-platform image')
  assert.ok(tests < amd64 && amd64 < arm64 && arm64 < gate && gate < login && login < publish)
  const arm64Step = source.slice(arm64, source.indexOf('\n      - name:', arm64 + 1))
  for (const line of ['platforms: linux/arm64', 'load: true', 'push: false', 'tags: openreader-ci-arm64:${{ github.sha }}'])
    assert.ok(arm64Step.includes(line), line)
  assert.ok(!source.slice(tests, publish).includes('continue-on-error'))
})
test('runtime Dockerfile installs only the exact signed zlib, musl and CA bundle fixes at build time', async () => {
  const source = await readFile(new URL('../Dockerfile', import.meta.url), 'utf8')
  const runtime = source.slice(source.lastIndexOf('\nFROM '))
  assert.ok(runtime.startsWith(`\nFROM ${baseline.baseImage}\n`))
  const apk = runtime.split('\n').filter(line => /\bapk\b/.test(line) && !line.trimStart().startsWith('#'))
  assert.deepEqual(apk, ["RUN apk add --no-cache 'zlib=1.4-r0' 'musl=1.2.5-r24' 'musl-utils=1.2.5-r24' 'ca-certificates-bundle=20261020-r0'"])
  const code = runtime.split('\n').filter(line => !line.trimStart().startsWith('#')).join('\n')
  assert.ok(!/--allow-untrusted|\bupgrade\b|--repository|\s-X\s|--force|:latest/.test(code))
  assert.ok(runtime.includes('CMD ["/app/openreader"]'))
})

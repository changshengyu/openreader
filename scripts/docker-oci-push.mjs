#!/usr/bin/env node

import { createHash } from 'node:crypto'
import { spawn } from 'node:child_process'
import { createReadStream } from 'node:fs'
import { mkdtemp, readFile, readdir, rm, stat } from 'node:fs/promises'
import { homedir, tmpdir } from 'node:os'
import { basename, join } from 'node:path'

function usage() {
  console.error('Usage: node scripts/docker-oci-push.mjs --archive <oci.tar> --image <registry/repository> --tag <tag> [--tag <tag>] [--remove-archive]')
  process.exit(2)
}

function parseArgs(argv) {
  const options = { tags: [], removeArchive: false }
  for (let index = 0; index < argv.length; index += 1) {
    const value = argv[index]
    if (value === '--remove-archive') {
      options.removeArchive = true
      continue
    }
    if (!['--archive', '--image', '--tag'].includes(value)) usage()
    const next = argv[index + 1]
    if (!next) usage()
    index += 1
    if (value === '--archive') options.archive = next
    if (value === '--image') options.image = next
    if (value === '--tag') options.tags.push(next)
  }
  if (!options.archive || !options.image || options.tags.length === 0) usage()
  return options
}

function splitImage(image) {
  const [registry, ...path] = image.split('/')
  if (!registry?.includes('.') || path.length === 0 || path.some(part => !part)) {
    throw new Error('image must include an explicit registry and repository')
  }
  return { registry, repository: path.join('/') }
}

function canonicalRegistry(value) {
  return String(value || '')
    .replace(/^https?:\/\//, '')
    .replace(/\/.*$/, '')
}

function run(command, args, input = '') {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { stdio: ['pipe', 'pipe', 'pipe'] })
    const stdout = []
    const stderr = []
    child.stdout.on('data', chunk => stdout.push(chunk))
    child.stderr.on('data', chunk => stderr.push(chunk))
    child.on('error', () => reject(new Error(`${command} could not be started`)))
    child.on('close', code => {
      if (code === 0) {
        resolve(Buffer.concat(stdout).toString('utf8'))
        return
      }
      reject(new Error(`${command} exited with status ${code}: ${Buffer.concat(stderr).toString('utf8').trim()}`))
    })
    child.stdin.end(input)
  })
}

async function dockerCredential(registry) {
  const configPath = join(process.env.DOCKER_CONFIG || join(homedir(), '.docker'), 'config.json')
  let config
  try {
    config = JSON.parse(await readFile(configPath, 'utf8'))
  } catch {
    throw new Error('Docker credential configuration is unavailable')
  }

  const configuredServers = [
    ...Object.keys(config.auths || {}),
    ...Object.keys(config.credHelpers || {}),
  ]
  const server = configuredServers.find(candidate => canonicalRegistry(candidate) === registry) || `https://${registry}`
  const inlineAuth = Object.entries(config.auths || {}).find(([candidate]) => canonicalRegistry(candidate) === registry)?.[1]?.auth
  if (inlineAuth) {
    const decoded = Buffer.from(inlineAuth, 'base64').toString('utf8')
    const separator = decoded.indexOf(':')
    if (separator > 0 && separator < decoded.length - 1) {
      return { username: decoded.slice(0, separator), secret: decoded.slice(separator + 1) }
    }
  }

  const helper = config.credHelpers?.[server] || config.credHelpers?.[registry] || config.credsStore
  if (!helper) throw new Error(`no Docker credential helper is configured for ${registry}`)
  let output
  try {
    output = await run(`docker-credential-${helper}`, ['get'], `${server}\n`)
  } catch {
    throw new Error(`Docker credential helper could not read credentials for ${registry}`)
  }
  let credential
  try {
    credential = JSON.parse(output)
  } catch {
    throw new Error(`Docker credential helper returned invalid credentials for ${registry}`)
  }
  if (!credential?.Username || !credential?.Secret) {
    throw new Error(`Docker credential helper returned empty credentials for ${registry}`)
  }
  return { username: credential.Username, secret: credential.Secret }
}

async function registryToken(registry, repository) {
  const credential = await dockerCredential(registry)
  const authorization = Buffer.from(`${credential.username}:${credential.secret}`).toString('base64')
  const tokenURL = new URL(`https://${registry}/token`)
  tokenURL.searchParams.set('service', registry)
  tokenURL.searchParams.set('scope', `repository:${repository}:pull,push`)
  let response
  try {
    response = await fetch(tokenURL, {
      headers: { Authorization: `Basic ${authorization}` },
      redirect: 'manual',
    })
  } catch {
    throw new Error(`could not request a registry token from ${registry}`)
  }
  if (!response.ok) throw new Error(`registry token request failed with status ${response.status}`)
  const payload = await response.json()
  const token = payload.token || payload.access_token
  if (!token) throw new Error('registry token response did not include an access token')
  return token
}

function sha256File(file) {
  return new Promise((resolve, reject) => {
    const hash = createHash('sha256')
    const stream = createReadStream(file)
    stream.on('data', chunk => hash.update(chunk))
    stream.on('error', reject)
    stream.on('end', () => resolve(hash.digest('hex')))
  })
}

function blobPath(root, digest) {
  const [algorithm, value] = String(digest || '').split(':', 2)
  if (algorithm !== 'sha256' || !/^[a-f0-9]{64}$/i.test(value || '')) {
    throw new Error('OCI layout contains an invalid sha256 descriptor')
  }
  return join(root, 'blobs', algorithm, value)
}

async function assertDescriptor(root, descriptor) {
  const file = blobPath(root, descriptor?.digest)
  const actual = await sha256File(file)
  if (actual !== descriptor.digest.slice('sha256:'.length)) {
    throw new Error(`OCI descriptor integrity check failed for ${descriptor.digest}`)
  }
  const info = await stat(file)
  if (Number.isFinite(descriptor.size) && info.size !== descriptor.size) {
    throw new Error(`OCI descriptor size check failed for ${descriptor.digest}`)
  }
  return file
}

function apiURL(registry, repository, suffix) {
  return `https://${registry}/v2/${repository}${suffix}`
}

function requestHeaders(token, extra = {}) {
  return {
    Authorization: `Bearer ${token}`,
    'User-Agent': 'OpenReader-local-oci-publisher',
    ...extra,
  }
}

async function uploadBlob(root, registry, repository, token, digest) {
  const file = blobPath(root, digest)
  const actual = await sha256File(file)
  if (`sha256:${actual}` !== digest) throw new Error(`OCI blob integrity check failed for ${digest}`)

  let response
  try {
    response = await fetch(apiURL(registry, repository, `/blobs/${digest}`), {
      method: 'HEAD',
      headers: requestHeaders(token),
      redirect: 'manual',
    })
  } catch {
    throw new Error(`registry blob check failed for ${digest}`)
  }
  if (response.status === 200) return false
  if (response.status !== 404) throw new Error(`registry blob check failed with status ${response.status}`)

  response = await fetch(apiURL(registry, repository, '/blobs/uploads/'), {
    method: 'POST',
    headers: requestHeaders(token),
    redirect: 'manual',
  })
  if (response.status !== 202) throw new Error(`registry blob upload could not start (status ${response.status})`)
  const location = response.headers.get('location')
  if (!location) throw new Error('registry blob upload did not return a location')

  const uploadURL = new URL(location, `https://${registry}`)
  uploadURL.searchParams.set('digest', digest)
  const info = await stat(file)
  response = await fetch(uploadURL, {
    method: 'PUT',
    headers: requestHeaders(token, {
      'Content-Type': 'application/octet-stream',
      'Content-Length': String(info.size),
    }),
    body: createReadStream(file),
    duplex: 'half',
    redirect: 'manual',
  })
  if (response.status !== 201) throw new Error(`registry blob upload failed with status ${response.status}`)
  return true
}

async function putManifest(registry, repository, token, reference, descriptor, body) {
  let response
  try {
    response = await fetch(apiURL(registry, repository, `/manifests/${encodeURIComponent(reference)}`), {
      method: 'PUT',
      headers: requestHeaders(token, {
        'Content-Type': descriptor.mediaType || 'application/vnd.oci.image.manifest.v1+json',
        'Content-Length': String(body.length),
      }),
      body,
      duplex: 'half',
      redirect: 'manual',
    })
  } catch {
    throw new Error(`registry manifest upload failed for ${reference}`)
  }
  if (![200, 201, 202].includes(response.status)) {
    throw new Error(`registry manifest upload failed with status ${response.status}`)
  }
}

async function extractArchive(archive) {
  const entries = (await run('tar', ['-tf', archive])).split('\n').filter(Boolean)
  const allowed = /^(oci-layout|index\.json|blobs|blobs\/sha256|blobs\/sha256\/[a-f0-9]{64})$/
  if (entries.length === 0 || entries.some(entry => !allowed.test(entry.replace(/\/$/, '')))) {
    throw new Error('OCI archive contains an unsupported or unsafe path')
  }
  const root = await mkdtemp(join(tmpdir(), 'openreader-oci-'))
  try {
    await run('tar', ['-xf', archive, '-C', root])
    return root
  } catch (error) {
    await rm(root, { recursive: true, force: true })
    throw error
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  const { registry, repository } = splitImage(options.image)
  const root = await extractArchive(options.archive)
  let published = false
  try {
    const index = JSON.parse(await readFile(join(root, 'index.json'), 'utf8'))
    if (index?.schemaVersion !== 2 || !Array.isArray(index.manifests) || index.manifests.length !== 1) {
      throw new Error('OCI archive must contain exactly one root image index descriptor')
    }
    const rootDescriptor = index.manifests[0]
    const token = await registryToken(registry, repository)
    const blobDir = join(root, 'blobs', 'sha256')
    const blobNames = (await readdir(blobDir)).filter(name => /^[a-f0-9]{64}$/i.test(name)).sort()
    for (const name of blobNames) {
      await uploadBlob(root, registry, repository, token, `sha256:${name}`)
    }

    const registered = new Set()
    const registerDescriptor = async (descriptor) => {
      if (!descriptor?.digest || registered.has(descriptor.digest)) return
      const file = await assertDescriptor(root, descriptor)
      const body = await readFile(file)
      let payload
      try {
        payload = JSON.parse(body)
      } catch {
        throw new Error(`OCI manifest ${descriptor.digest} is not valid JSON`)
      }
      for (const child of payload.manifests || []) await registerDescriptor(child)
      if (payload.subject?.digest) {
        const subject = (payload.manifests || []).find(item => item.digest === payload.subject.digest)
        if (subject) await registerDescriptor(subject)
      }
      await putManifest(registry, repository, token, descriptor.digest, descriptor, body)
      registered.add(descriptor.digest)
    }

    const rootFile = await assertDescriptor(root, rootDescriptor)
    const rootBody = await readFile(rootFile)
    const rootPayload = JSON.parse(rootBody)
    for (const descriptor of rootPayload.manifests || []) await registerDescriptor(descriptor)
    for (const tag of [...new Set(options.tags)]) await putManifest(registry, repository, token, tag, rootDescriptor, rootBody)
    published = true
    console.log(`OCI publish complete: ${options.image}:${[...new Set(options.tags)].join(', ')}`)
  } finally {
    await rm(root, { recursive: true, force: true })
    if (options.removeArchive && published) await rm(options.archive, { force: true })
  }
}

main().catch(error => {
  console.error(`OCI publish failed: ${error.message}`)
  process.exit(1)
})

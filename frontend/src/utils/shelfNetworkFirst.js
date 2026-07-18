export async function resolveShelfNetworkFirst({
  request,
  readFallback,
  isCurrent = () => true,
  hasCurrent = () => false,
}) {
  let networkError
  try {
    const value = await request()
    if (!isCurrent()) return { source: 'discarded' }
    return { source: 'network', value }
  } catch (error) {
    networkError = error
  }

  if (!isCurrent()) return { source: 'discarded' }
  if (hasCurrent()) return { source: 'current' }

  const value = await readFallback()
  if (!isCurrent()) return { source: 'discarded' }
  if (hasCurrent()) return { source: 'current' }
  if (value === null || value === undefined) throw networkError
  return { source: 'fallback', value }
}

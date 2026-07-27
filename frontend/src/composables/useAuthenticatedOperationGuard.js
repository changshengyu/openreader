import { getCurrentScope, onScopeDispose } from 'vue'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

export function useAuthenticatedOperationGuard(options = {}) {
  const operations = createAuthenticatedOperationGuard({
    getIdentity: options.getIdentity,
  })
  const eventTarget = options.eventTarget === undefined
    ? (typeof window === 'undefined' ? null : window)
    : options.eventTarget
  let disposed = false

  function invalidateSession() {
    operations.reset()
  }

  function dispose() {
    if (disposed) return
    disposed = true
    eventTarget?.removeEventListener?.(
      'openreader:session-invalidated',
      invalidateSession,
    )
    operations.reset()
  }

  eventTarget?.addEventListener?.(
    'openreader:session-invalidated',
    invalidateSession,
  )
  if (getCurrentScope()) onScopeDispose(dispose)

  return {
    ...operations,
    dispose,
  }
}

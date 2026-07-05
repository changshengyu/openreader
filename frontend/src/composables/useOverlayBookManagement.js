import { useOverlayBookBatchActions } from './useOverlayBookBatchActions.js'
import { useOverlayBookItemActions } from './useOverlayBookItemActions.js'

export function useOverlayBookManagement(options) {
  const batchActions = useOverlayBookBatchActions(options)
  const itemActions = useOverlayBookItemActions(options, {
    batchBusy: batchActions.batchBusy,
  })

  return {
    ...batchActions,
    ...itemActions,
  }
}

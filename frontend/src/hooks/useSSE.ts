import { useEffect, useRef } from 'react'

type SSEHandler<T> = (data: T) => void

interface SSEHandlers {
  cluster_summary?: SSEHandler<unknown>
  nodes?: SSEHandler<unknown>
  alerts?: SSEHandler<unknown>
  connected?: SSEHandler<unknown>
}

/**
 * useSSE connects to the backend SSE stream and dispatches events to handlers.
 * Reconnects automatically on connection loss (exponential backoff, max 30s).
 */
export function useSSE(handlers: SSEHandlers) {
  const handlersRef = useRef(handlers)
  handlersRef.current = handlers  // always use latest handlers without re-subscribing

  useEffect(() => {
    let es: EventSource | null = null
    let retryDelay = 1000
    let unmounted = false

    function connect() {
      if (unmounted) return
      es = new EventSource('/api/v1/stream')

      es.addEventListener('connected', () => {
        retryDelay = 1000 // reset backoff on successful connect
        handlersRef.current.connected?.({})
      })

      es.addEventListener('cluster_summary', (e) => {
        try { handlersRef.current.cluster_summary?.(JSON.parse(e.data)) }
        catch { /* ignore parse errors */ }
      })

      es.addEventListener('nodes', (e) => {
        try { handlersRef.current.nodes?.(JSON.parse(e.data)) }
        catch { /* ignore parse errors */ }
      })

      es.addEventListener('alerts', (e) => {
        try { handlersRef.current.alerts?.(JSON.parse(e.data)) }
        catch { /* ignore parse errors */ }
      })

      es.onerror = () => {
        es?.close()
        if (!unmounted) {
          setTimeout(connect, retryDelay)
          retryDelay = Math.min(retryDelay * 2, 30_000)
        }
      }
    }

    connect()
    return () => {
      unmounted = true
      es?.close()
    }
  }, []) // intentionally empty — handlers are accessed via ref
}

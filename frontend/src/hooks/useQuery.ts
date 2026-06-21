import { useEffect, useReducer, useRef } from 'react'

type State<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; data: T }
  | { status: 'error'; error: string }

type Action<T> =
  | { type: 'fetch' }
  | { type: 'success'; data: T }
  | { type: 'error'; error: string }

function reducer<T>(state: State<T>, action: Action<T>): State<T> {
  switch (action.type) {
    case 'fetch':   return { status: 'loading' }
    case 'success': return { status: 'success', data: action.data }
    case 'error':   return { status: 'error', error: action.error }
    default:        return state
  }
}

/**
 * useQuery fetches data on mount and whenever `key` changes.
 * Optionally re-fetches every `pollMs` milliseconds for pages
 * that don't have SSE coverage.
 */
export function useQuery<T>(
  fn: (signal: AbortSignal) => Promise<T>,
  key: string | string[],
  options: { pollMs?: number } = {}
) {
  const [state, dispatch] = useReducer(reducer<T>, { status: 'loading' })
  const keyStr = Array.isArray(key) ? key.join('|') : key
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    let active = true

    async function run() {
      dispatch({ type: 'fetch' })
      try {
        const data = await fn(controller.signal)
        if (active) dispatch({ type: 'success', data })
      } catch (err) {
        if (active && !controller.signal.aborted) {
          dispatch({ type: 'error', error: err instanceof Error ? err.message : String(err) })
        }
      }
    }

    run()

    if (options.pollMs) {
      pollRef.current = setInterval(run, options.pollMs)
    }

    return () => {
      active = false
      controller.abort()
      if (pollRef.current) clearInterval(pollRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [keyStr, options.pollMs])

  return state
}

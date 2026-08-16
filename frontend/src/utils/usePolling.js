
import { useEffect, useRef } from 'react'
import { POLL_INTERVAL_MS } from '../auth'

export { POLL_INTERVAL_MS }

export function usePolling(fetcher, { intervalMs = POLL_INTERVAL_MS, enabled = true, immediate = true } = {}) {
  const fetcherRef = useRef(fetcher)
  fetcherRef.current = fetcher
  const runningRef = useRef(false)

  useEffect(() => {
    if (!enabled) return undefined

    let timer = null
    let cancelled = false

    const tick = async () => {
      if (runningRef.current || cancelled) return
      runningRef.current = true
      try {
        await fetcherRef.current()
      } catch {
        
      } finally {
        runningRef.current = false
      }
    }

    const schedule = () => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(async () => {
        if (cancelled) return
        await tick()
        if (!cancelled) schedule() 
      }, intervalMs)
    }

    if (immediate) tick()
    schedule()

    const onVisible = () => {
      if (document.visibilityState === 'visible') {
        tick()
        schedule()
      } else if (timer) {
        clearTimeout(timer)
        timer = null
      }
    }
    document.addEventListener('visibilitychange', onVisible)

    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [intervalMs, enabled, immediate])
}
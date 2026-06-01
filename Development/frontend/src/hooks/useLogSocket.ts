import { useCallback, useEffect, useRef, useState } from 'react'

export interface LogEntry {
  ts: string
  level: 'info' | 'warn' | 'error'
  message: string
}

const MAX_ENTRIES = 100

export function useLogSocket(enabled = true) {
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [connected, setConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    if (!enabled) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | null = null

    function connect() {
      if (cancelled) return
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      const ws = new WebSocket(`${proto}://${location.host}/ws/log`)
      wsRef.current = ws

      ws.onopen = () => { if (!cancelled) setConnected(true) }

      ws.onmessage = (event) => {
        if (cancelled) return
        try {
          const msg = JSON.parse(event.data)
          if (msg.action === 'log') {
            setEntries(prev => {
              const next = [...prev, { ts: msg.ts, level: msg.level as LogEntry['level'], message: msg.message }]
              return next.length > MAX_ENTRIES ? next.slice(-MAX_ENTRIES) : next
            })
          }
        } catch { /* ignore malformed */ }
      }

      ws.onclose = () => {
        if (cancelled) return
        setConnected(false)
        timer = setTimeout(connect, 2000)
      }

      ws.onerror = () => {
        ws.onclose = null
        if (cancelled) return
        setConnected(false)
        timer = setTimeout(connect, 2000)
      }
    }

    connect()

    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      if (wsRef.current) {
        wsRef.current.onclose = null
        wsRef.current.onerror = null
        wsRef.current.close()
        wsRef.current = null
      }
    }
  }, [enabled])

  const send = useCallback((msg: object) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg))
    }
  }, [])

  const clear = useCallback(() => {
    setEntries([])
    send({ action: 'clear_log' })
  }, [send])

  return { entries, connected, clear }
}

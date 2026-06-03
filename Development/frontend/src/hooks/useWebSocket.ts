import { useCallback, useEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import type { WsMessage } from '@/lib/types'

export function useWebSocket(channel: string, enabled = true) {
  const [lastMessage, setLastMessage] = useState<WsMessage | null>(null)
  const [connected, setConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    if (!enabled) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | null = null

    function connect() {
      if (cancelled) return

      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      const ws = new WebSocket(`${proto}://${location.host}/ws/${channel}`)
      wsRef.current = ws

      ws.onopen = () => {
        if (!cancelled) setConnected(true)
      }

      ws.onmessage = (event) => {
        if (cancelled) return
        try {
          // flushSync verhindert React-18-Batching: jede Nachricht bekommt
          // einen eigenen Render-Zyklus, damit keine Nachricht durch
          // back-to-back Nachrichten (sync + kiosk_state) übersprungen wird.
          flushSync(() => {
            setLastMessage(JSON.parse(event.data) as WsMessage)
          })
        } catch { /* ignore malformed */ }
      }

      ws.onclose = () => {
        if (cancelled) return
        setConnected(false)
        if (timer) clearTimeout(timer)
        timer = setTimeout(connect, 2000)
      }

      ws.onerror = () => {
        ws.onclose = null
        if (cancelled) return
        setConnected(false)
        if (timer) clearTimeout(timer)
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
  }, [channel, enabled])

  const send = useCallback((msg: WsMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg))
    }
  }, [])

  return { lastMessage, send, connected }
}

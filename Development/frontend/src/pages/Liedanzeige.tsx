import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useWebSocket } from '@/hooks/useWebSocket'
import { DEFAULTS, FONTS, type DisplaySettings } from '@/lib/types'
import { STORAGE_KEY, loadSettings } from '@/lib/settings'

// ── Subkomponenten ────────────────────────────────────────────────────────────

function ClockFace({ style }: { style: React.CSSProperties }) {
  const [time, setTime] = useState(() => new Date().toTimeString().slice(0, 5))
  const wrapRef  = useRef<HTMLDivElement>(null)
  const textRef  = useRef<HTMLDivElement>(null)
  // CSS-Variable-Referenz als Basis für die Messung merken
  const baseSize = useRef<string>((style.fontSize as string) ?? '')

  useEffect(() => {
    const id = setInterval(() => setTime(new Date().toTimeString().slice(0, 5)), 1000)
    return () => clearInterval(id)
  }, [])

  // Passt font-size (px) so an, dass der Text nie breiter als der Wrapper ist.
  // Funktioniert unabhängig von Schrift und Viewport: kein transform, kein clip-Problem.
  const rescale = useCallback(() => {
    const wrap = wrapRef.current
    const text = textRef.current
    if (!wrap || !text) return
    // Zur CSS-Variable zurücksetzen, dann natürliche Breite messen
    text.style.fontSize = baseSize.current
    const available = wrap.offsetWidth
    const natural   = text.scrollWidth
    if (natural > available && natural > 0) {
      const base = parseFloat(getComputedStyle(text).fontSize)
      text.style.fontSize = `${Math.floor(base * available / natural)}px`
    }
  }, [])

  // baseSize-Ref vor rescale aktualisieren (Hooks laufen in Definitionsreihenfolge)
  useLayoutEffect(() => { if (style.fontSize) baseSize.current = style.fontSize as string }, [style.fontSize])
  useLayoutEffect(rescale, [rescale, style, time])

  useEffect(() => {
    const obs = new ResizeObserver(rescale)
    if (wrapRef.current) obs.observe(wrapRef.current)
    return () => obs.disconnect()
  }, [rescale])

  return (
    <div ref={wrapRef} style={{ width: '95vw', height: style.fontSize, overflow: 'hidden', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div
        ref={textRef}
        className="font-bold tabular-nums"
        style={{ ...style, lineHeight: 1, whiteSpace: 'nowrap', display: 'inline-flex', alignItems: 'center' }}
      >
        {time.split('').map((ch, i) => <span key={i}>{ch}</span>)}
      </div>
    </div>
  )
}

function formatDate(d: Date) {
  return `${String(d.getDate()).padStart(2, '0')}.${String(d.getMonth() + 1).padStart(2, '0')}.${d.getFullYear()}`
}

function DateLine() {
  const [date, setDate] = useState(() => formatDate(new Date()))

  useEffect(() => {
    const now = new Date()
    const next = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
    let intervalId: ReturnType<typeof setInterval> | null = null

    const timeout = setTimeout(() => {
      setDate(formatDate(new Date()))
      intervalId = setInterval(() => setDate(formatDate(new Date())), 86_400_000)
    }, +next - +now)

    return () => {
      clearTimeout(timeout)
      if (intervalId) clearInterval(intervalId)
    }
  }, [])

  return (
    <div
      className="text-center font-bold"
      style={{ fontSize: 'var(--date-font-size)', lineHeight: 1, textShadow: 'var(--text-shadow)' }}
    >
      {date}
    </div>
  )
}

// ── Hauptkomponente ───────────────────────────────────────────────────────────

export default function Liedanzeige({ kanal }: { kanal: string }) {
  const [settings, setSettings] = useState<DisplaySettings>(loadSettings)
  const [inputNumbers, setInputNumbers] = useState('')
  const [showOverlay, setShowOverlay] = useState(false)
  const [everConnected, setEverConnected] = useState(false)
  const [blackout, setBlackout] = useState(false)

  useEffect(() => {
    (window as any).__kioskBlackout = (show: boolean) => setBlackout(show)
    return () => { delete (window as any).__kioskBlackout }
  }, [])

  const { lastMessage, connected, send } = useWebSocket(kanal)

  useEffect(() => {
    document.title = kanal === 'lied' ? 'Liedanzeige' : 'Choranzeige'
  }, [kanal])

  // Overlay nur zeigen wenn Verbindung nach erfolgreichem Connect abbricht —
  // nicht beim ersten Laden (der Kiosk-Overlay deckt diesen Fall ab).
  useEffect(() => {
    if (connected) {
      setEverConnected(true)
      setShowOverlay(false)
      return
    }
    if (!everConnected) return
    const id = setTimeout(() => setShowOverlay(true), 800)
    return () => clearTimeout(id)
  }, [connected, everConnected])

  // CSS-Variablen anwenden
  useEffect(() => {
    const r = document.documentElement.style
    const fontObj = FONTS.find(f => f.key === settings.font) ?? FONTS[0]
    r.setProperty('--time-small-font-size', `${settings.timeSize}vw`)
    r.setProperty('--time-large-font-size', `${Math.round(settings.timeSize * 0.74)}vw`)
    r.setProperty('--date-font-size',       `${Math.round(settings.timeSize * 0.52)}vw`)
    r.setProperty('--gap-time-date',        `${settings.gapTimeDate}px`)
    r.setProperty('--font-family',          fontObj.value)
    const opacity = (settings.shadowStrength / 100).toFixed(2)
    r.setProperty('--text-shadow',          `10px 10px 12px rgba(0,0,0,${opacity})`)
  }, [settings])

  // Auto-Reset-Timer: React-idiomatisch via Effect-Cleanup.
  // Läuft nur wenn Zahlen angezeigt werden; wird automatisch gecleant wenn
  // inputNumbers → '' oder resetDelay sich ändert.
  useEffect(() => {
    if (inputNumbers.length === 0) return
    const id = setTimeout(() => setInputNumbers(''), settings.resetDelay * 60 * 1000)
    return () => clearTimeout(id)
  }, [inputNumbers, settings.resetDelay])

  // USB-Numpad (nur Chor-Kanal, Fallback für Browser-Tests ohne Kiosk):
  // Im Kiosk übernimmt der globale Go-Hook in numpad.go — dort werden Tasten
  // bereits geschluckt und erreichen diesen Handler nicht mehr.
  useEffect(() => {
    if (kanal !== 'chor') return

    function onKeyDown(e: KeyboardEvent) {
      // Numpad-Tasten: location === 3; Sondertasten (Calc, Media, …): eigene e.code-Präfixe
      const isNumpad = e.location === 3 || e.code === 'NumLock'
      const isExtra = /^(Launch|Media|Audio|Browser|Sleep|WakeUp|Power)/.test(e.code)
      if (!isNumpad && !isExtra) return

      if (/^Numpad[0-9]$/.test(e.code)) {
        send({ action: 'input', key: e.code.slice(-1), target: 'chor' })
      } else {
        send({ action: 'reset', target: 'chor' })
      }
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [kanal, send])

  // Pure Updater — keine Seiteneffekte im setState-Callback
  const handleInput = useCallback((key: string) => {
    setInputNumbers(prev => prev.length >= 4 ? prev : prev + key)
  }, [])

  const handleBackspace = useCallback(() => {
    setInputNumbers(prev => prev.slice(0, -1))
  }, [])

  useEffect(() => {
    if (!lastMessage) return
    const msg = lastMessage
    if (msg.action === 'input')     { handleInput(msg.key) }
    if (msg.action === 'backspace') { handleBackspace() }
    if (msg.action === 'reset')     { setInputNumbers('') }
    if (msg.action === 'settings')  {
      const s = { ...DEFAULTS, ...msg.settings }
      setSettings(s)
      localStorage.setItem(STORAGE_KEY, JSON.stringify(s))
    }
    if (msg.action === 'display') {
      setInputNumbers('')
      ;(msg.value ?? '').split('').forEach((ch: string) => handleInput(ch))
    }
  }, [lastMessage, handleInput, handleBackspace])

  const isInputMode = inputNumbers.length > 0

  const timeSmallStyle: React.CSSProperties = { fontSize: 'var(--time-small-font-size)', lineHeight: 1, textShadow: 'var(--text-shadow)' }
  const timeLargeStyle: React.CSSProperties = { fontSize: 'var(--time-large-font-size)', lineHeight: 1, textShadow: 'var(--text-shadow)' }

  return (
    <div
      className="flex flex-col justify-center items-center h-screen overflow-hidden select-none"
      style={{ backgroundColor: 'var(--background-color, white)', color: 'var(--font-color, black)', fontFamily: 'var(--font-family)', margin: 0 }}
    >
      {blackout && <div className="fixed inset-0 bg-black z-[9999]" />}

      {showOverlay && (
        <div className="fixed inset-0 bg-white flex flex-col items-center justify-center gap-6 z-50">
          <div className="w-14 h-14 rounded-full border-4 border-neutral-200 border-t-neutral-500 animate-spin" />
          <span className="font-sans text-base text-black animate-pulse">Verbinde mit Server…</span>
        </div>
      )}
      <div className="flex flex-col items-center" style={{ gap: 'var(--gap-time-date)' }}>

        {/* Uhrmodus */}
        {!isInputMode && (
          <div className="flex flex-col" style={{ width: 'fit-content', gap: 'var(--gap-time-date)' }}>
            <ClockFace style={timeSmallStyle} />
            <DateLine />
          </div>
        )}

        {/* Eingabemodus: Zahl + verkleinerte Uhr */}
        {isInputMode && (
          <>
            <div
              className="flex items-center justify-center font-bold"
              style={{ fontSize: 'var(--time-small-font-size)', height: 'var(--time-small-font-size)', lineHeight: 1, textShadow: 'var(--text-shadow)' }}
            >
              {inputNumbers}
            </div>
            <ClockFace style={timeLargeStyle} />
          </>
        )}

      </div>
    </div>
  )
}

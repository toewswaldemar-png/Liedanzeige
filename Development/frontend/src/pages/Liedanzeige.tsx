import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useWebSocket } from '@/hooks/useWebSocket'
import { DEFAULTS, FONTS, type DisplaySettings } from '@/lib/types'
import { STORAGE_KEY, loadSettings } from '@/lib/settings'

// ── Gemeinsamer Rescale-Hook ──────────────────────────────────────────────────
//
// scale   (0–100): Wie viel Prozent der verfügbaren Breite der Text einnehmen soll.
// fontKey (string): Font-Schlüssel aus den Settings — löst rescale bei Font-Wechsel aus.
//
// Algorithmus: Text auf 100 vw setzen → natürliche Breite messen → maxFit berechnen → scale anwenden.
// Kein Grenzbereich, kein Flip, kein Vibrations-Problem.
//
// Font-Handling:
//   - fontKey in useCallback-Deps → rescale wird neu erzeugt wenn Font wechselt
//     (gecachte Fonts: sofortige korrekte Messung)
//   - document.fonts.ready: behebt falsche Messung beim ersten Laden (Font noch nicht gecacht)
//   - document.fonts 'loadingdone': behebt falsche Messung bei neu geladenem Font nach Wechsel

function useScaledText(scale: number, fontKey: string, onHeight?: (h: number) => void, maxHeightPx?: number) {
  const wrapRef    = useRef<HTMLDivElement>(null)
  const textRef    = useRef<HTMLDivElement>(null)
  // Optionaler Mess-Anker: wenn belegt, wird dieses Element für die Breitenmessung
  // genutzt (statt textRef). Ermöglicht fixen Referenz-Inhalt unabhängig vom angezeigten Text.
  const measureRef = useRef<HTMLDivElement>(null)

  const rescale = useCallback(() => {
    const wrap  = wrapRef.current
    const text  = textRef.current
    if (!wrap || !text) return
    // measureRef nutzen wenn vorhanden, sonst textRef
    const probe = measureRef.current ?? textRef.current
    if (!probe) return
    probe.style.fontSize = '100vw'
    void probe.offsetWidth
    const available = wrap.offsetWidth
    const natural   = probe.scrollWidth
    if (natural <= 0 || available <= 0) return
    const ref100 = parseFloat(getComputedStyle(probe).fontSize) // px-Wert von 100vw
    const maxFit = ref100 * available / natural                  // Schriftgröße die genau 100% füllt
    const widthScaled = Math.floor(maxFit * scale / 100)
    const scaled = maxHeightPx !== undefined ? Math.min(widthScaled, maxHeightPx) : widthScaled
    text.style.fontSize  = `${scaled}px`
    probe.style.fontSize = `${scaled}px`  // probe zurücksetzen
    wrap.style.height    = `${scaled}px`
    onHeight?.(scaled)                    // Höhe nach oben melden (für Gap-Berechnung)
  }, [scale, fontKey, onHeight, maxHeightPx])  // fontKey als Dep → rescale wird bei Font-Wechsel neu erzeugt

  useEffect(() => {
    window.addEventListener('resize', rescale)
    return () => window.removeEventListener('resize', rescale)
  }, [rescale])

  useEffect(() => {
    // Gecachte Fonts: useLayoutEffect reicht. Nicht-gecachte Fonts: nach Laden nochmal messen.
    document.fonts.ready.then(rescale)
    document.fonts.addEventListener('loadingdone', rescale)
    return () => document.fonts.removeEventListener('loadingdone', rescale)
  }, [rescale])

  return { wrapRef, textRef, measureRef, rescale }
}

// ── Subkomponenten ────────────────────────────────────────────────────────────

function ClockFace({ style, scale, fontKey, sizeRef, onHeight, maxHeightPx }: { style: React.CSSProperties; scale: number; fontKey: string; sizeRef?: string; onHeight?: (h: number) => void; maxHeightPx?: number }) {
  const [time, setTime] = useState(() => new Date().toTimeString().slice(0, 5))
  const { wrapRef, textRef, measureRef, rescale } = useScaledText(scale, fontKey, onHeight, maxHeightPx)

  useEffect(() => {
    const id = setInterval(() => setTime(new Date().toTimeString().slice(0, 5)), 1000)
    return () => clearInterval(id)
  }, [])

  // Nur bei scale-Änderung rescalen — time-Änderung ist mit tabular-nums immer gleich breit
  useLayoutEffect(rescale, [rescale])

  return (
    <div ref={wrapRef} style={{ width: '95vw', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div
        ref={textRef}
        className="font-bold tabular-nums"
        style={{ ...style, lineHeight: 1, whiteSpace: 'nowrap', display: 'inline-flex', alignItems: 'center' }}
      >
        {time.split('').map((ch, i) => <span key={i}>{ch}</span>)}
      </div>
      {sizeRef && (
        <div ref={measureRef} aria-hidden className="font-bold tabular-nums"
          style={{ ...style, lineHeight: 1, whiteSpace: 'nowrap', visibility: 'hidden', position: 'absolute', pointerEvents: 'none' }}>
          {sizeRef}
        </div>
      )}
    </div>
  )
}

function formatDate(d: Date) {
  return `${String(d.getDate()).padStart(2, '0')}.${String(d.getMonth() + 1).padStart(2, '0')}.${d.getFullYear()}`
}

function DateLine({ style, scale, fontKey, onHeight, maxHeightPx }: { style: React.CSSProperties; scale: number; fontKey: string; onHeight?: (h: number) => void; maxHeightPx?: number }) {
  const [date, setDate] = useState(() => formatDate(new Date()))
  const { wrapRef, textRef, rescale } = useScaledText(scale, fontKey, onHeight, maxHeightPx)

  useEffect(() => {
    const now = new Date()
    const next = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
    let intervalId: ReturnType<typeof setInterval> | null = null
    const timeout = setTimeout(() => {
      setDate(formatDate(new Date()))
      intervalId = setInterval(() => setDate(formatDate(new Date())), 86_400_000)
    }, +next - +now)
    return () => { clearTimeout(timeout); if (intervalId) clearInterval(intervalId) }
  }, [])

  useLayoutEffect(rescale, [rescale])

  return (
    <div ref={wrapRef} style={{ width: '95vw', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div
        ref={textRef}
        className="font-bold tabular-nums"
        style={{ ...style, lineHeight: 1, whiteSpace: 'nowrap' }}
      >
        {date}
      </div>
    </div>
  )
}

function NumberDisplay({ value, style, scale, fontKey, onHeight, maxHeightPx }: { value: string; style: React.CSSProperties; scale: number; fontKey: string; onHeight?: (h: number) => void; maxHeightPx?: number }) {
  const { wrapRef, textRef, measureRef, rescale } = useScaledText(scale, fontKey, onHeight, maxHeightPx)

  // Nur bei scale/fontKey rescalen — nicht bei jedem Tastendruck.
  // Größe basiert auf dem Referenz-Inhalt "0000", nicht auf der aktuellen Eingabe.
  useLayoutEffect(rescale, [rescale])

  return (
    <div ref={wrapRef} style={{ width: '95vw', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div
        ref={textRef}
        className="font-bold tabular-nums"
        style={{ ...style, lineHeight: 1, whiteSpace: 'nowrap' }}
      >
        {value}
      </div>
      {/* Referenz "00:00" → selbe Breite wie Uhrzeit "HH:MM" → Zahl gleich groß wie Uhrzeit im Uhrmodus */}
      <div
        ref={measureRef}
        aria-hidden
        className="font-bold tabular-nums"
        style={{ ...style, lineHeight: 1, whiteSpace: 'nowrap', visibility: 'hidden', position: 'absolute', pointerEvents: 'none' }}
      >
        00:00
      </div>
    </div>
  )
}

// ── Hauptkomponente ───────────────────────────────────────────────────────────

export default function Liedanzeige({ kanal }: { kanal: string }) {
  const [settings, setSettings] = useState<DisplaySettings>(loadSettings)
  const [inputNumbers, setInputNumbers] = useState('')
  const isInputMode = inputNumbers.length > 0
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

  // CSS-Variablen — font-size wird von den Komponenten selbst gesetzt; gap via applyGap
  useEffect(() => {
    const r = document.documentElement.style
    const fontObj = FONTS.find(f => f.key === settings.font) ?? FONTS[0]
    r.setProperty('--font-family', fontObj.value)
    const opacity = (settings.shadowStrength / 100).toFixed(2)
    // em-Einheiten: CSS-Variable wird an der Verwendungsstelle aufgelöst →
    // Schatten skaliert proportional zur Schriftgröße des jeweiligen Elements
    r.setProperty('--text-shadow', `0.025em 0.025em 0.04em rgba(0,0,0,${opacity})`)
  }, [settings])

  // ── Prozentualer Abstand ──────────────────────────────────────────────────────
  // Abstand = gapTimeDate% × (viewport_height − h1 − h2)
  // Alle Werte in Refs → keine Re-Render-Schleife.
  const heightsRef  = useRef({ clockClock: 0, clockDate: 0, inputNum: 0, inputClock: 0 })
  const gapPctRef   = useRef(settings.gapTimeDate)
  const modeRef     = useRef(false) // isInputMode

  const [viewportH, setViewportH] = useState(window.innerHeight)
  useEffect(() => {
    const onResize = () => setViewportH(window.innerHeight)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  // Höhengrenzen: Elemente teilen den Viewport proportional zu ihrer Skalierung
  const clockModeMaxH = Math.floor(viewportH / 2)
  const inputTotal    = settings.timeSize + settings.subClockSize
  const inputNumMaxH  = inputTotal > 0 ? Math.floor(viewportH * settings.timeSize    / inputTotal) : Math.floor(viewportH / 2)
  const inputClkMaxH  = inputTotal > 0 ? Math.floor(viewportH * settings.subClockSize / inputTotal) : Math.floor(viewportH / 2)

  const applyGap = useCallback(() => {
    const h = heightsRef.current
    const [h1, h2] = modeRef.current
      ? [h.inputNum, h.inputClock]
      : [h.clockClock, h.clockDate]
    if (h1 <= 0 || h2 <= 0) return   // Höhen noch nicht bekannt
    const remaining = Math.max(0, window.innerHeight - h1 - h2)
    const gapPx = Math.floor(remaining * gapPctRef.current / 100)
    document.documentElement.style.setProperty('--gap-time-date', `${gapPx}px`)
  }, [])

  // Gap bei Prozent-Änderung neu berechnen
  useEffect(() => {
    gapPctRef.current = settings.gapTimeDate
    applyGap()
  }, [settings.gapTimeDate, applyGap])

  // Gap bei Modus-Wechsel neu berechnen
  useEffect(() => {
    modeRef.current = isInputMode
    applyGap()
  }, [isInputMode, applyGap])

  // Gap bei Viewport-Größenänderung neu berechnen
  useEffect(() => {
    window.addEventListener('resize', applyGap)
    return () => window.removeEventListener('resize', applyGap)
  }, [applyGap])

  // Callbacks für Höhen-Updates aus den Komponenten
  const onClockClockH = useCallback((h: number) => { heightsRef.current.clockClock = h; applyGap() }, [applyGap])
  const onClockDateH  = useCallback((h: number) => { heightsRef.current.clockDate  = h; applyGap() }, [applyGap])
  const onInputNumH   = useCallback((h: number) => { heightsRef.current.inputNum   = h; applyGap() }, [applyGap])
  const onInputClockH = useCallback((h: number) => { heightsRef.current.inputClock = h; applyGap() }, [applyGap])

  useEffect(() => {
    if (inputNumbers.length === 0) return
    const id = setTimeout(() => setInputNumbers(''), settings.resetDelay * 60 * 1000)
    return () => clearTimeout(id)
  }, [inputNumbers, settings.resetDelay])

  useEffect(() => {
    if (kanal !== 'chor') return
    function onKeyDown(e: KeyboardEvent) {
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

  const baseStyle: React.CSSProperties = { lineHeight: 1, textShadow: 'var(--text-shadow)' }

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
          <div className="flex flex-col items-center" style={{ gap: 'var(--gap-time-date)' }}>
            <ClockFace style={baseStyle} scale={settings.timeSize} fontKey={settings.font} onHeight={onClockClockH} maxHeightPx={clockModeMaxH} />
            <DateLine  style={baseStyle} scale={settings.timeSize} fontKey={settings.font} onHeight={onClockDateH}  maxHeightPx={clockModeMaxH} />
          </div>
        )}

        {/* Eingabemodus: Zahl + verkleinerte Uhr */}
        {isInputMode && (
          <>
            <NumberDisplay value={inputNumbers} style={baseStyle} scale={settings.timeSize}    fontKey={settings.font} onHeight={onInputNumH}   maxHeightPx={inputNumMaxH} />
            <ClockFace                          style={baseStyle} scale={settings.subClockSize} fontKey={settings.font} onHeight={onInputClockH} maxHeightPx={inputClkMaxH} />
          </>
        )}

      </div>
    </div>
  )
}

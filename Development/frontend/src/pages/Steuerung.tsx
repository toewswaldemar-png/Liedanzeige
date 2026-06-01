import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { BookOpenText, ExternalLink, Maximize2, Minimize2, Monitor, RotateCw, Settings2, Terminal, Trash2, X } from 'lucide-react'
import { useWebSocket } from '@/hooks/useWebSocket'
import { useLogSocket } from '@/hooks/useLogSocket'
import { LogPanel } from '@/components/LogPanel'
import { Button } from '@/components/ui/button'
import { Slider } from '@/components/ui/slider'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { FONTS, type DisplaySettings } from '@/lib/types'
import { STORAGE_KEY, loadSettings } from '@/lib/settings'

function formatMinutes(min: number) {
  return min === 1 ? '1 Minute' : `${min} Minuten`
}

const NUMPAD_KEYS = ['7', '8', '9', '4', '5', '6', '1', '2', '3']

// ── Einstellungs-Sektion ──────────────────────────────────────────────────────
function SettingsSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-xl bg-white border border-zinc-200 shadow-sm">
      <div className="px-4 pt-3 pb-1">
        <p className="text-[10px] font-bold uppercase tracking-widest text-zinc-400">{title}</p>
      </div>
      <div className="px-4 pb-4 flex flex-col gap-4">
        {children}
      </div>
    </div>
  )
}

// ── Slider-Zeile ──────────────────────────────────────────────────────────────
function SliderRow({
  label, value, min, max, step, sliderValue, onChange,
}: {
  label: string; value: string; min: number; max: number; step: number
  sliderValue: number; onChange: (v: number) => void
}) {
  return (
    <div className="flex flex-col gap-0">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-zinc-500">{label}</span>
        <span className="text-xs tabular-nums font-mono text-zinc-400">{value}</span>
      </div>
      <div className="relative">
        <div className="absolute inset-x-0 top-1/2 -translate-y-1/2 border-t border-zinc-200 pointer-events-none" />
        <Slider
          min={min} max={max} step={step}
          value={[sliderValue]}
          onValueChange={v => onChange(Array.isArray(v) ? v[0] : v)}
        />
      </div>
    </div>
  )
}

export default function Steuerung({ kanal }: { kanal: 'lied' | 'chor' }) {
  const navigate = useNavigate()
  const target = kanal

  const [display, setDisplay] = useState('')
  const [settings, setSettings] = useState<DisplaySettings>(loadSettings)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [resetProgress, setResetProgress] = useState(100)
  const resetStartRef = useRef<number | null>(null)
  const resetIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Fix: send vor toggleKanal deklariert
  const { send, connected, lastMessage } = useWebSocket('steuerung')

  useEffect(() => {
    document.title = kanal === 'lied' ? 'Steuerung – Lied' : 'Steuerung – Chor'
  }, [kanal])

  const toggleKanal = () => {
    send({ action: 'reset', target: 'lied' })
    setDisplay('')
    navigate(target === 'lied' ? '/steuerung/chor' : '/steuerung/lied', { replace: true })
  }

  useEffect(() => {
    if (resetIntervalRef.current) clearInterval(resetIntervalRef.current)
    if (display.length === 0 || target !== 'lied') {
      setResetProgress(100)
      resetStartRef.current = null
      return
    }
    resetStartRef.current = Date.now()
    const totalMs = settings.resetDelay * 60 * 1000
    resetIntervalRef.current = setInterval(() => {
      if (!resetStartRef.current) return
      const remaining = 1 - (Date.now() - resetStartRef.current) / totalMs
      if (remaining <= 0) {
        setResetProgress(0)
        clearInterval(resetIntervalRef.current!)
        send({ action: 'reset', target: 'lied' })
      } else {
        setResetProgress(remaining * 100)
      }
    }, 200)
    return () => clearInterval(resetIntervalRef.current!)
  // Fix: send in Dependency-Array
  }, [display, settings.resetDelay, target, send])

  // Beim ersten Verbindungsaufbau beide Kanäle zurücksetzen
  const didReset = useRef(false)
  useEffect(() => {
    if (connected && !didReset.current) {
      didReset.current = true
      send({ action: 'reset', target: 'lied' })
      send({ action: 'reset', target: 'chor' })
    }
  }, [connected, send])

  const updateSetting = useCallback(<K extends keyof DisplaySettings>(key: K, value: DisplaySettings[K]): void => {
    setSettings(prev => {
      const next = { ...prev, [key]: value }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
      send({ action: 'settings', settings: next })
      return next
    })
  }, [send])

  const handleKey = useCallback((key: string) => {
    if (display.length >= 4) return
    send({ action: 'input', key, target })
  }, [display, target, send])

  const handleLoeschen = useCallback(() => {
    send({ action: 'reset', target: 'lied' })
  }, [send])

  // Display vom Server-Echo treiben — alle Steuerung-Tabs bleiben synchron
  useEffect(() => {
    if (!lastMessage) return
    const msg = lastMessage
    if (msg.action === 'input')      setDisplay(prev => prev.length >= 4 ? prev : prev + msg.key)
    if (msg.action === 'backspace')  setDisplay(prev => prev.slice(0, -1))
    if (msg.action === 'reset')      setDisplay('')
    if (msg.action === 'kiosk_state') setIsFullscreen(msg.fullscreen)
  }, [lastMessage])

  const [isFullscreen, setIsFullscreen] = useState(true)
  const kioskCmd = (command: string) => send({ action: 'kiosk', command })

  const [logOpen, setLogOpen] = useState(false)
  const { entries: logEntries, clear: clearLog } = useLogSocket()

  return (
    <div className="flex flex-col h-svh bg-background">

      {/* ── Header ── */}
      <header className="flex items-center justify-between px-4 border-b shrink-0 h-14" onClick={() => setSettingsOpen(false)}>
        <nav className="flex items-center gap-2 text-xs font-semibold tracking-widest select-none">
          <button
            onClick={toggleKanal}
            className="flex items-center gap-3 text-muted-foreground active:text-foreground transition-colors cursor-pointer"
          >
            <BookOpenText className="w-4 h-4" />
            {target === 'lied' ? 'LIEDANZEIGE' : 'CHORANZEIGE'}
          </button>
          <span className="text-muted-foreground">·</span>
          <div className="flex items-center gap-1.5 font-normal text-muted-foreground select-none">
            <span className={cn('w-2 h-2 rounded-full', connected ? 'bg-green-500' : 'bg-red-500')} />
            {connected ? 'Verbunden' : 'Verbinde…'}
          </div>
        </nav>

        <div className="flex items-center gap-2">
          {target === 'lied' && (
            <>
              <button
                onClick={e => { e.stopPropagation(); setLogOpen(v => !v) }}
                className={cn(
                  'inline-flex items-center justify-center h-8 w-8 rounded-lg border transition-colors',
                  logOpen
                    ? 'bg-zinc-900 text-white border-zinc-700 hover:bg-zinc-700 hover:text-white'
                    : 'border-input bg-background hover:bg-accent hover:text-accent-foreground'
                )}
              >
                <Terminal className="w-4 h-4" />
              </button>
              <button
                onClick={e => { e.stopPropagation(); setSettingsOpen(v => !v) }}
                className={cn(
                  'inline-flex items-center justify-center h-8 w-8 rounded-lg border transition-colors',
                  settingsOpen
                    ? 'bg-zinc-900 text-white border-zinc-700 hover:bg-zinc-700 hover:text-white'
                    : 'border-input bg-background hover:bg-accent hover:text-accent-foreground'
                )}
              >
                <Settings2 className="w-4 h-4" />
              </button>
            </>
          )}
        </div>
      </header>

      {/* ── Inhaltsbereich ── */}
      <div className="flex flex-1 flex-col items-center p-4 gap-3 overflow-hidden">
        <div className="flex flex-col gap-3 w-full max-w-md flex-1">

          {/* Display */}
          <div className={cn(
            'rounded-xl border-2 px-6 py-4 transition-colors duration-200 shrink-0',
            display.length > 0
              ? 'border-blue-500 bg-blue-50'
              : 'border-input bg-transparent'
          )}>
            <div className="flex items-center justify-center gap-6">
              {Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="w-14 h-20 flex items-center justify-center select-none">
                  {display[i]
                    ? <span className="text-7xl font-semibold tabular-nums text-zinc-900 leading-none">{display[i]}</span>
                    : <span className="w-10 h-0.5 rounded-full bg-muted-foreground/30" />
                  }
                </div>
              ))}
            </div>
          </div>

          {/* Auto-Reset Fortschrittsbalken (nur Liedanzeige) */}
          {target === 'lied' && (
            <div className="h-1.5 rounded-full bg-zinc-100 overflow-hidden shrink-0">
              <div
                className="h-full rounded-full bg-blue-400 transition-[width] duration-200 ease-linear"
                style={{ width: `${resetProgress}%` }}
              />
            </div>
          )}

          {/* Numpad */}
          <div className="grid grid-cols-3 gap-2 flex-1" style={{ gridTemplateRows: 'repeat(4, 1fr)' }}>
            {NUMPAD_KEYS.map(k => (
              <Button
                key={k}
                variant="outline"
                className="h-full text-4xl font-normal select-none touch-manipulation
                  transition-all duration-75
                  hover:border-blue-200 hover:bg-blue-50/60
                  active:scale-95 active:bg-blue-600 active:text-white active:border-blue-600"
                onClick={() => handleKey(k)}
              >
                {k}
              </Button>
            ))}
            <Button
              variant="outline"
              className="h-full text-4xl font-normal select-none touch-manipulation
                transition-all duration-75
                hover:border-blue-200 hover:bg-blue-50/60
                active:scale-95 active:bg-blue-600 active:text-white active:border-blue-600"
              onClick={() => handleKey('0')}
            >
              0
            </Button>
            <Button
              variant="ghost"
              className="col-span-2 h-full gap-2 text-sm font-semibold tracking-widest select-none touch-manipulation rounded-md border
                bg-red-50 text-red-600 border-red-300
                hover:bg-red-100 hover:text-red-700 hover:border-red-400
                transition-all duration-75
                active:scale-95 active:bg-red-600 active:text-white active:border-red-600"
              onClick={handleLoeschen}
            >
              <Trash2 className="w-4 h-4" />
              LÖSCHEN
            </Button>
          </div>

        </div>
      </div>

      {/* ── Log-Panel ── */}
      {target === 'lied' && logOpen && <LogPanel entries={logEntries} onClear={clearLog} />}

      {/* ── Einstellungs-Panel ── */}
      {settingsOpen && (
        <>
          {/* Backdrop */}
          <div
            className="fixed inset-0 z-40"
            style={{ top: '3.5rem' }}
            onClick={() => setSettingsOpen(false)}
          />

          {/* Panel */}
          <div
            className="fixed right-0 bottom-0 w-80 bg-zinc-50 border-l border-zinc-200 shadow-2xl z-50 flex flex-col"
            style={{ top: '3.5rem' }}
          >
            {/* Panel-Header */}
            <div className="flex items-center justify-between px-4 h-12 border-b border-zinc-200 bg-white shrink-0">
              <div className="flex items-center gap-2 text-zinc-800">
                <Settings2 className="w-4 h-4 text-zinc-400" />
                <span className="font-semibold text-sm">Einstellungen</span>
              </div>
              <button
                onClick={() => setSettingsOpen(false)}
                className="flex items-center justify-center w-7 h-7 rounded-md text-zinc-400 hover:text-zinc-700 hover:bg-zinc-100 transition-colors"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Scrollbarer Inhalt */}
            <div className="flex-1 overflow-y-auto p-3 flex flex-col gap-3">

              {/* ── Darstellung ── */}
              <SettingsSection title="Darstellung">
                <SliderRow
                  label="Schriftgröße" value={`${settings.timeSize} vw`}
                  min={10} max={45} step={1} sliderValue={settings.timeSize}
                  onChange={v => updateSetting('timeSize', v)}
                />
                <SliderRow
                  label="Abstand Uhrzeit–Datum" value={`${settings.gapTimeDate} px`}
                  min={0} max={100} step={2} sliderValue={settings.gapTimeDate}
                  onChange={v => updateSetting('gapTimeDate', v)}
                />
                <SliderRow
                  label="Schatten" value={`${settings.shadowStrength} %`}
                  min={0} max={100} step={5} sliderValue={settings.shadowStrength}
                  onChange={v => updateSetting('shadowStrength', v)}
                />
                <SliderRow
                  label="Auto-Reset" value={formatMinutes(settings.resetDelay)}
                  min={1} max={10} step={1} sliderValue={settings.resetDelay}
                  onChange={v => updateSetting('resetDelay', v)}
                />
              </SettingsSection>

              {/* ── Schrift ── */}
              <SettingsSection title="Schrift">
                <Select value={settings.font} onValueChange={v => { if (v) updateSetting('font', v) }}>
                  <SelectTrigger className="h-9 w-48 bg-white border-zinc-200 text-zinc-800">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {FONTS.map(f => <SelectItem key={f.key} value={f.key}>{f.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </SettingsSection>

              {/* ── Anzeigefenster ── */}
              <SettingsSection title="Anzeigefenster">
                <div className="grid grid-cols-2 gap-2">
                  <button
                    onClick={() => {
                      const next = !isFullscreen
                      setIsFullscreen(next)
                      kioskCmd(next ? 'fullscreen' : 'windowed')
                    }}
                    className="flex flex-col items-center gap-1.5 rounded-lg border border-zinc-200 bg-white px-2 py-3 text-xs font-medium text-zinc-700 hover:bg-zinc-50 hover:border-zinc-300 active:bg-zinc-100 transition-colors"
                  >
                    {isFullscreen ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
                    {isFullscreen ? 'Fenster' : 'Vollbild'}
                  </button>
                  {[
                    { label: 'Reload',  icon: <RotateCw className="w-4 h-4" />, cmd: 'reload'       },
                    { label: 'Monitor', icon: <Monitor  className="w-4 h-4" />, cmd: 'swap_monitors' },
                  ].map(({ label, icon, cmd }) => (
                    <button
                      key={cmd}
                      onClick={() => kioskCmd(cmd)}
                      className="flex flex-col items-center gap-1.5 rounded-lg border border-zinc-200 bg-white px-2 py-3 text-xs font-medium text-zinc-700 hover:bg-zinc-50 hover:border-zinc-300 active:bg-zinc-100 transition-colors"
                    >
                      {icon}
                      {label}
                    </button>
                  ))}
                </div>
              </SettingsSection>

              {/* ── Im Browser oeffnen ── */}
              <SettingsSection title="Im Browser">
                <div className="grid grid-cols-2 gap-2">
                  {[
                    { label: 'Liedanzeige', href: '/lied'  },
                    { label: 'Choranzeige', href: '/chor'  },
                  ].map(({ label, href }) => (
                    <button
                      key={href}
                      onClick={() => window.open(href, '_blank')}
                      className="flex flex-col items-center gap-1.5 rounded-lg border border-zinc-200 bg-white px-2 py-3 text-xs font-medium text-zinc-700 hover:bg-zinc-50 hover:border-zinc-300 active:bg-zinc-100 transition-colors"
                    >
                      <ExternalLink className="w-4 h-4" />
                      {label}
                    </button>
                  ))}
                </div>
              </SettingsSection>

            </div>
          </div>
        </>
      )}

    </div>
  )
}

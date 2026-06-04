import { useEffect, useRef } from 'react'
import { Trash2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { LogEntry } from '@/hooks/useLogSocket'

interface Props {
  entries: LogEntry[]
  onClear: () => void
}

function levelColor(level: LogEntry['level']) {
  if (level === 'warn')  return 'text-amber-400'
  if (level === 'error') return 'text-red-400'
  return 'text-zinc-300'
}

export function LogPanel({ entries, onClear }: Props) {
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [entries])

  return (
    <div className="border-t border-zinc-800 bg-zinc-950 flex flex-col shrink-0 h-44">
      {/* Header */}
      <div className="flex items-center justify-between px-3 h-7 border-b border-zinc-800 shrink-0">
        <span className="text-[10px] font-bold uppercase tracking-widest text-zinc-500 font-sans select-none">
          Server-Log
        </span>
        <button
          onClick={onClear}
          className="text-zinc-600 hover:text-zinc-400 transition-colors"
          title="Leeren"
        >
          <Trash2 className="w-3 h-3" />
        </button>
      </div>

      {/* Eintraege */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto p-2 flex flex-col gap-0.5">
        {entries.length === 0 && (
          <span className="text-zinc-600 text-xs font-mono">Keine Eintraege.</span>
        )}
        {entries.map((e, i) => (
          <div key={i} className={cn('flex gap-2 text-xs font-mono leading-snug', levelColor(e.level))}>
            <span className="text-zinc-600 shrink-0 flex flex-col leading-tight">
              {e.ts.split(' ').map((part, i) => <span key={i}>{part}</span>)}
            </span>
            <span className="break-all">{e.message}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

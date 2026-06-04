export interface DisplaySettings {
  timeSize: number
  subClockSize: number
  gapTimeDate: number
  font: string
  resetDelay: number
  shadowStrength: number
}

export type WsMessage =
  | { action: 'input'; key: string; target?: string; steuerungState?: string }
  | { action: 'backspace'; target?: string }
  | { action: 'reset'; target?: string }
  | { action: 'settings'; settings: DisplaySettings }
  | { action: 'display'; value: string }
  | { action: 'kiosk'; command: string }
  | { action: 'kiosk_state'; fullscreen: boolean }
  | { action: 'log'; level: 'info' | 'warn' | 'error'; message: string; ts: string }
  | { action: 'sync'; liedState: string; chorState: string; steuerungState?: string; settings?: DisplaySettings }

export const FONTS = [
  { key: 'segoe-ui',      label: 'Segoe UI',      value: '"Segoe UI", sans-serif'      },
  { key: 'arial',         label: 'Arial',          value: 'Arial, sans-serif'           },
  { key: 'verdana',       label: 'Verdana',        value: 'Verdana, sans-serif'         },
  { key: 'trebuchet-ms',  label: 'Trebuchet MS',   value: '"Trebuchet MS", sans-serif'  },
  { key: 'georgia',       label: 'Georgia',        value: 'Georgia, serif'              },
  { key: 'times',         label: 'Times New Roman', value: '"Times New Roman", serif'   },
] as const

export const DEFAULTS: DisplaySettings = {
  timeSize: 75,
  subClockSize: 75,
  gapTimeDate: 0,
  font: 'segoe-ui',
  resetDelay: 5,
  shadowStrength: 40,
}

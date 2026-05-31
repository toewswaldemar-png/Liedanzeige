import { DEFAULTS, type DisplaySettings } from './types'

export const STORAGE_KEY = 'liedanzeige-settings'

export function loadSettings(): DisplaySettings {
  try {
    return { ...DEFAULTS, ...JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') }
  } catch {
    return { ...DEFAULTS }
  }
}

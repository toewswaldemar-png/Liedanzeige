import { useEffect } from 'react'

export function useFavicon(href: string) {
  useEffect(() => {
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    if (!link) return
    const prev = link.href
    link.href = href
    return () => { link.href = prev }
  }, [href])
}

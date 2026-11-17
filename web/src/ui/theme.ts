// Thème clair / sombre (ADR 0034). Le choix explicite de l'utilisateur est enregistré par navigateur
// (localStorage) ; sans choix, la préférence du système (prefers-color-scheme) s'applique. Le stockage
// peut être indisponible (navigation privée, quota, politique du navigateur) : jamais d'erreur, le thème
// s'applique simplement pour la session.
import { useCallback, useEffect, useState } from 'react'

export type Theme = 'light' | 'dark'

export const THEME_KEY = 'bom-studio.theme'

export function storedTheme(): Theme | null {
  try {
    const v = window.localStorage.getItem(THEME_KEY)
    return v === 'light' || v === 'dark' ? v : null
  } catch {
    return null
  }
}

function systemTheme(): Theme {
  try {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  } catch {
    return 'light'
  }
}

export function initialTheme(): Theme {
  return storedTheme() ?? systemTheme()
}

export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme
}

function saveTheme(theme: Theme) {
  try {
    window.localStorage.setItem(THEME_KEY, theme)
  } catch {
    // stockage indisponible : le choix vaut pour la session seulement
  }
}

// useTheme : thème courant et bascule. Tant que l'utilisateur n'a rien choisi, un changement de la
// préférence système est suivi.
export function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = useState<Theme>(initialTheme)
  useEffect(() => applyTheme(theme), [theme])
  useEffect(() => {
    let mq: MediaQueryList
    try {
      mq = window.matchMedia('(prefers-color-scheme: dark)')
    } catch {
      return
    }
    const onChange = () => {
      if (storedTheme() === null) setTheme(mq.matches ? 'dark' : 'light')
    }
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])
  const toggle = useCallback(() => {
    setTheme((t) => {
      const next: Theme = t === 'dark' ? 'light' : 'dark'
      saveTheme(next)
      return next
    })
  }, [])
  return [theme, toggle]
}

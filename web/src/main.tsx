import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './ui/tokens.css'
import './index.css'
import App from './App.tsx'
import { applyTheme, initialTheme } from './ui/theme.ts'

// Thème posé avant le premier rendu : pas d'éclair de thème clair pour qui a choisi le sombre.
applyTheme(initialTheme())

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)

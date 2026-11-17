// Montage de la page de démo /vehicle3d-demo.html (lot D, ADR 0035).
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Demo } from './Demo.tsx'

createRoot(document.getElementById('root')!).render(<StrictMode><Demo /></StrictMode>)

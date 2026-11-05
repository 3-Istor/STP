import { useEffect, useState } from 'react'
import BomPage from './bom/BomPage.tsx'
import SuppliersPage from './suppliers/SuppliersPage.tsx'

// Deux espaces : la BOM (par défaut) et les fournisseurs. L'espace actif est dans l'ancre de l'URL
// (#boms, #fournisseurs) pour qu'un lien ou un rechargement y revienne.
type Space = 'boms' | 'fournisseurs'

function spaceFromHash(): Space {
  return window.location.hash === '#fournisseurs' ? 'fournisseurs' : 'boms'
}

export default function App() {
  const [space, setSpace] = useState<Space>(spaceFromHash)

  useEffect(() => {
    const onHash = () => setSpace(spaceFromHash())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  return (
    <>
      <nav className="space-nav" aria-label="Espaces">
        <a href="#boms" aria-current={space === 'boms' ? 'page' : undefined}>BOM</a>
        <a href="#fournisseurs" aria-current={space === 'fournisseurs' ? 'page' : undefined}>Fournisseurs</a>
      </nav>
      {space === 'boms' ? <BomPage /> : <SuppliersPage />}
    </>
  )
}

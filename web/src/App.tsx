import { useEffect, useState } from 'react'

export default function App() {
  const [api, setApi] = useState('…')

  useEffect(() => {
    fetch('/healthz')
      .then((r) => (r.ok ? 'en ligne' : `erreur ${r.status}`))
      .catch(() => 'injoignable')
      .then(setApi)
  }, [])

  return (
    <main>
      <h1>BOM Studio</h1>
      <p>API : {api}</p>
    </main>
  )
}

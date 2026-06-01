import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import Liedanzeige from '@/pages/Liedanzeige'
import Steuerung from '@/pages/Steuerung'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/lied"           element={<Liedanzeige kanal="lied" />} />
        <Route path="/chor"           element={<Liedanzeige kanal="chor" />} />
        <Route path="/steuerung"      element={<Navigate to="/steuerung/lied" replace />} />
        <Route path="/steuerung/lied" element={<Steuerung kanal="lied" />} />
        <Route path="/steuerung/chor" element={<Steuerung kanal="chor" />} />
      </Routes>
    </BrowserRouter>
  )
}

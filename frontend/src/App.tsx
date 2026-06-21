import { Routes, Route } from 'react-router-dom'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { Nodes } from './pages/Nodes'
import { Jobs } from './pages/Jobs'
import { Alerts } from './pages/Alerts'
import { Assistant } from './pages/Assistant'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/"          element={<Dashboard />} />
        <Route path="/nodes"     element={<Nodes />} />
        <Route path="/jobs"      element={<Jobs />} />
        <Route path="/alerts"    element={<Alerts />} />
        <Route path="/assistant" element={<Assistant />} />
      </Route>
    </Routes>
  )
}

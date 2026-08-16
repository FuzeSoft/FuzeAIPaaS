import React, { useState, useEffect, useRef } from 'react'
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import AutoML from './pages/AutoML'
import AutoMLStudy from './pages/AutoMLStudy'
import Sidebar from './components/Sidebar'
import Dashboard from './pages/Dashboard'
import Models from './pages/Models'
import Datasets from './pages/Datasets'
import Inference from './pages/Inference'
import InferenceAccel from './pages/InferenceAccel'
import Workspaces from './pages/Workspaces'
import Training from './pages/Training'
import Experiments from './pages/Experiments'
import ExperimentDetail from './pages/ExperimentDetail'
import ExperimentCompare from './pages/ExperimentCompare'
import Evaluations from './pages/Evaluations'
import EvaluationReport from './pages/EvaluationReport'
import Resources from './pages/Resources'
import Clusters from './pages/Clusters'
import Monitoring from './pages/Monitoring'
import Alerts from './pages/Alerts'
import LLMOps from './pages/LLMOps'
import AgentStudio from './pages/AgentStudio'
import Edge from './pages/Edge'
import Tools from './pages/Tools'
import Login from './pages/Login'
import Settings from './pages/Settings'
import IdPAdmin from './pages/IdPAdmin'
import {
  getToken,
  getUser,
  setUser,
  getSession,
  invalidateSessionCache,
  logout,
} from './auth'

function MainApp({ onLogout }) {
  return (
    <Router>
      <div className="flex min-h-screen bg-slate-950">
        <Sidebar onLogout={onLogout} />
        <div className="flex-1 ml-64">
          <Routes>
            <Route path="/" element={<Navigate to="/models" replace />} />
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/models" element={<Models />} />
            <Route path="/datasets" element={<Datasets />} />
            <Route path="/inference" element={<Inference />} />
            <Route path="/inference-accel" element={<InferenceAccel />} />
            <Route path="/workspaces" element={<Workspaces />} />
            <Route path="/training" element={<Training />} />
            <Route path="/experiments" element={<Experiments />} />
            <Route path="/experiments/compare" element={<ExperimentCompare />} />
            <Route path="/experiments/:id" element={<ExperimentDetail />} />
            <Route path="/automl" element={<AutoML />} />
            <Route path="/automl/:id" element={<AutoMLStudy />} />
            <Route path="/evaluations" element={<Evaluations />} />
            <Route path="/evaluations/:id" element={<EvaluationReport />} />
            {}
            <Route path="/jobs" element={<Navigate to="/training" replace />} />
            <Route path="/resources" element={<Resources />} />
            <Route path="/clusters" element={<Clusters />} />
            <Route path="/llmops" element={<LLMOps />} />
            <Route path="/agents" element={<AgentStudio />} />
            <Route path="/edge" element={<Edge />} />
            <Route path="/tools" element={<Tools />} />
            <Route path="/monitoring" element={<Monitoring />} />
            <Route path="/alerts" element={<Alerts />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/admin/idps" element={<IdPAdmin />} />
            <Route path="*" element={<Navigate to="/models" replace />} />
          </Routes>
        </div>
      </div>
    </Router>
  )
}

function LoadingScreen() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-950 text-slate-400">
      加载中…
    </div>
  )
}

async function handleLogout() {
  try {
    await logout() 
  } finally {
    invalidateSessionCache()
    window.location.replace('/login')
  }
}

function AuthGuard() {
  const [status, setStatus] = useState('loading')
  const checkedRef = useRef(false)

  useEffect(() => {
    if (checkedRef.current) return
    checkedRef.current = true
    let cancelled = false

    const boot = async () => {
      const localToken = getToken()
      if (localToken) {
        
        const cached = getUser()
        if (!cancelled) setUser(cached)
        const me = await getSession()
        if (cancelled) return
        if (me) {
          setUser(me)
          setStatus('authed')
        } else {
          
          localStorage.removeItem('fuze_token')
          localStorage.removeItem('fuze_user')
          invalidateSessionCache()
          setStatus('guest')
        }
        return
      }

      const me = await getSession()
      if (cancelled) return
      if (me) {
        setUser(me)
        setStatus('authed')
      } else {
        setStatus('guest')
      }
    }
    boot()
    return () => { cancelled = true }
  }, [])

  if (status === 'loading') {
    return <LoadingScreen />
  }

  if (status === 'guest') {
    return (
      <Router>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </Router>
    )
  }

  return (
    <Router>
      <Routes>
        <Route path="/login" element={<Navigate to="/" replace />} />
        <Route path="*" element={<MainApp onLogout={handleLogout} />} />
      </Routes>
    </Router>
  )
}

function App() {
  return <AuthGuard />
}

export default App
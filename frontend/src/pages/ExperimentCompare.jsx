import React, { useState, useEffect, useCallback } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts'
import { apiFetch } from '../auth'
import { runStatusStyle } from '../utils/status'
import { ArrowLeft, GitCompareArrows, CheckCircle, AlertCircle, Clock } from 'lucide-react'

const parseJSON = (raw) => {
  if (!raw) return {}
  if (typeof raw === 'object') return raw
  try {
    const v = JSON.parse(raw)
    return v && typeof v === 'object' ? v : {}
  } catch {
    return {}
  }
}
const fmt = (n) => (typeof n === 'number' ? Number(n.toFixed(4)) : n)

const ExperimentCompare = () => {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [experiments, setExperiments] = useState([])
  const [selected, setSelected] = useState(() => {
    const ids = searchParams.get('ids')
    return ids ? ids.split(',').filter(Boolean) : []
  })
  const [metricName, setMetricName] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [result, setResult] = useState(null)

  useEffect(() => {
    apiFetch('/api/v1/experiments').then((res) => res.json()).then((data) => {
      const list = Array.isArray(data) ? data : []
      setExperiments(list)
      if (list.length) setMetricName(list[0].metric_name || '')
    }).catch(() => setExperiments([]))
  }, [])

  const canCompare = selected.length >= 2 && metricName

  const runCompare = useCallback(async () => {
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const params = new URLSearchParams({ ids: selected.join(','), metric_name: metricName })
      const res = await apiFetch(`/api/v1/experiments/compare?${params.toString()}`)
      const data = await res.json()
      setResult(data)
    } catch (err) {
      const detail = err.message || '对比失败'
      setError(detail)
    } finally {
      setLoading(false)
    }
  }, [selected, metricName])

  const [runsMap, setRunsMap] = useState({})
  useEffect(() => {
    if (!result) return
    let cancelled = false
    const ids = result.experiments.map((e) => e.id)
    Promise.all(ids.map((eid) =>
      apiFetch(`/api/v1/experiments/${eid}/runs`).then((r) => r.json()).then((runs) => ({ id: eid, runs: Array.isArray(runs) ? runs : [] }))
        .catch(() => ({ id: eid, runs: [] }))
    )).then((entries) => {
      if (cancelled) return
      const map = {}
      entries.forEach((e) => { map[e.id] = e.runs })
      setRunsMap(map)
    })
    return () => { cancelled = true }
  }, [result])

  const trendData = (() => {
    if (!result) return []
    const all = []
    result.experiments.forEach((e) => {
      const runs = (runsMap[e.id] || []).slice().sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
      runs.forEach((run) => {
        if (typeof run.metric_value !== 'number') return
        all.push({ time: new Date(run.created_at).toLocaleString(), [e.name]: fmt(run.metric_value) })
      })
    })
    return all
  })()

  const trendLines = result ? result.experiments.map((e) => ({ key: e.name, color: pickColor(e.id) })) : []

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <button onClick={() => navigate('/experiments')} className="inline-flex items-center gap-2 text-slate-400 hover:text-white mb-6">
          <ArrowLeft className="w-4 h-4" />返回实验列表
        </button>

        <div className="mb-8">
          <h1 className="text-3xl font-bold text-white mb-2">跨实验对比</h1>
          <p className="text-slate-400">仅支持对比目标指标名称（metric_name）相同的实验。差异高亮、最优 Run 并排展示，并叠加各实验趋势曲线。</p>
        </div>

        <div className="bg-slate-800 rounded-2xl border border-slate-700 p-6 mb-6">
          <div className="flex flex-wrap items-end gap-4">
            <div className="flex-1 min-w-[280px]">
              <label className="block text-sm font-medium text-slate-300 mb-2">选择实验</label>
              <div className="flex flex-wrap gap-2">
                {experiments.map((exp) => {
                  const active = selected.includes(exp.id)
                  return (
                    <button
                      key={exp.id}
                      onClick={() => setSelected((prev) => (active ? prev.filter((x) => x !== exp.id) : [...prev, exp.id]))}
                      className={`px-3 py-1.5 rounded-lg border text-sm transition-colors ${
                        active ? 'bg-blue-600 border-blue-600 text-white' : 'bg-slate-700 border-slate-600 text-slate-300 hover:border-blue-400'
                      }`}
                    >
                      {exp.name} · {exp.metric_name}
                    </button>
                  )
                })}
              </div>
            </div>
            <div className="w-48">
              <label className="block text-sm font-medium text-slate-300 mb-2">目标指标</label>
              <select value={metricName} onChange={(e) => setMetricName(e.target.value)} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                {[...new Set(experiments.map((e) => e.metric_name))].filter(Boolean).map((m) => (
                  <option key={m} value={m}>{m}</option>
                ))}
              </select>
            </div>
            <button
              onClick={runCompare}
              disabled={!canCompare || loading}
              className={`flex items-center gap-2 px-6 py-3 rounded-xl font-medium transition-all ${
                canCompare ? 'bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white' : 'bg-slate-700 text-slate-600 cursor-not-allowed'
              }`}
            >
              <GitCompareArrows className="w-5 h-5" />
              {loading ? '对比中…' : '开始对比'}
            </button>
          </div>
          {!canCompare && <p className="text-amber-400 text-sm mt-2">请至少选择 2 个实验并指定目标指标名称。</p>}
        </div>

        {error && (
          <div className="bg-red-500/10 border border-red-500/30 text-red-400 rounded-xl p-4 mb-6">{error}</div>
        )}

        {result && (
          <div className="space-y-6">
            <div className="bg-slate-800 rounded-2xl border border-slate-700 p-6">
              <h2 className="text-lg font-bold text-white mb-4">
                对比表（目标指标：{result.metric_name} · {result.objective === 'minimize' ? '越低越好' : '越高越好'}）
              </h2>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="bg-slate-900/60 text-slate-400">
                    <tr>
                      <th className="text-left font-medium px-4 py-3">实验</th>
                      <th className="text-left font-medium px-4 py-3">最优指标</th>
                      <th className="text-left font-medium px-4 py-3">最优 Run 超参</th>
                      <th className="text-left font-medium px-4 py-3">状态</th>
                    </tr>
                  </thead>
                  <tbody>
                    {result.experiments.map((exp) => {
                      const best = exp.best_run
                      const style = best ? runStatusStyle(best.status) : null
                      const hyper = best ? parseJSON(best.hyperparameters) : {}
                      const hyperText = Object.entries(hyper).map(([k, v]) => `${k}=${v}`).join(', ')
                      return (
                        <tr key={exp.id} className="border-t border-slate-700/60">
                          <td className="px-4 py-3 text-white">
                            <div className="flex items-center gap-2">
                              <span>{exp.name}</span>
                              {exp.is_overall_best && (
                                <span className="text-[11px] px-2 py-0.5 rounded-full bg-green-500/20 text-green-400">总体最优</span>
                              )}
                            </div>
                          </td>
                          <td className="px-4 py-3 text-white font-bold">{best ? fmt(best.metric_value) : '—'}</td>
                          <td className="px-4 py-3 text-slate-300 max-w-xs truncate" title={hyperText}>{hyperText || '—'}</td>
                          <td className="px-4 py-3">
                            {style && (
                              <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full ${style.bg} ${style.text}`}>{style.label}</span>
                            )}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="bg-slate-800 rounded-2xl border border-slate-700 p-6">
              <h2 className="text-lg font-bold text-white mb-4">趋势叠加</h2>
              {trendData.length ? (
                <ResponsiveContainer width="100%" height={320}>
                  <LineChart data={trendData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
                    <XAxis dataKey="time" stroke="#94a3b8" fontSize={12} />
                    <YAxis stroke="#94a3b8" fontSize={12} />
                    <Tooltip contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #334155', borderRadius: '0.75rem', color: '#fff' }} />
                    <Legend />
                    {trendLines.map((l) => (
                      <Line key={l.key} type="monotone" dataKey={l.key} stroke={l.color} strokeWidth={2} dot={{ r: 3 }} />
                    ))}
                  </LineChart>
                </ResponsiveContainer>
              ) : (
                <div className="text-center text-slate-500 py-12">暂无趋势数据。</div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

const PALETTE = ['#60a5fa', '#34d399', '#f472b6', '#fbbf24', '#a78bfa', '#22d3ee']
const COLOR_CACHE = {}
function pickColor(id) {
  if (!COLOR_CACHE[id]) {
    COLOR_CACHE[id] = PALETTE[Object.keys(COLOR_CACHE).length % PALETTE.length]
  }
  return COLOR_CACHE[id]
}

export default ExperimentCompare
import React, { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { apiFetch } from '../auth'
import { ArrowLeft, Play, Target, GitBranch, Trophy, Activity } from 'lucide-react'

const AutoMLStudy = () => {
  const { id } = useParams()
  const [study, setStudy] = useState(null)
  const [trials, setTrials] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => { fetchAll() }, [id])

  const fetchAll = async () => {
    try {
      const [sRes, tRes] = await Promise.all([
        apiFetch(`/api/v1/hpo/${id}`),
        apiFetch(`/api/v1/hpo/${id}/trials`),
      ])
      const sData = await sRes.json().catch(() => ({}))
      const tData = await tRes.json().catch(() => [])
      setStudy(sData)
      setTrials(Array.isArray(tData) ? tData : [])
    } catch (e) {
      console.error('Error fetching study:', e)
    } finally {
      setLoading(false)
    }
  }

  const handleRun = async () => {
    try {
      await apiFetch(`/api/v1/hpo/${id}/run`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })
      fetchAll()
    } catch (e) {
      console.error('Error running study:', e)
    }
  }

  const statusStyle = (s) => {
    if (s === 'completed') return 'text-green-300 bg-green-500/10'
    if (s === 'running') return 'text-blue-300 bg-blue-500/10'
    if (s === 'failed') return 'text-red-300 bg-red-500/10'
    if (s === 'pruned') return 'text-amber-300 bg-amber-500/10'
    if (s === 'cancelled') return 'text-slate-400 bg-slate-500/10'
    return 'text-slate-300 bg-slate-500/10'
  }

  if (loading) {
    return <div className="flex items-center justify-center min-h-screen"><div className="text-slate-400">加载中...</div></div>
  }
  if (!study) {
    return <div className="p-8 text-slate-400">研究不存在</div>
  }

  const num = (v) => (v === null || v === undefined ? '—' : Number(v).toFixed(4))

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <Link to="/automl" className="inline-flex items-center gap-2 text-slate-400 hover:text-white mb-6">
          <ArrowLeft className="w-4 h-4" /> 返回 AutoML
        </Link>

        <div className="flex items-start justify-between mb-8">
          <div className="flex items-center gap-4">
            <div className="w-14 h-14 bg-gradient-to-br from-purple-500 to-blue-600 rounded-2xl flex items-center justify-center">
              <GitBranch className="w-7 h-7 text-white" />
            </div>
            <div>
              <h1 className="text-3xl font-bold text-white mb-1">{study.name}</h1>
              <div className="flex items-center gap-3">
                <span className={`text-xs px-2 py-0.5 rounded-full ${statusStyle(study.status)}`}>{study.status}</span>
                <span className="text-slate-400 text-sm">算法 {study.algorithm || 'tpe'}</span>
              </div>
            </div>
          </div>
          <button onClick={handleRun} className="flex items-center gap-2 bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 text-white px-5 py-3 rounded-xl font-medium transition-all">
            <Play className="w-4 h-4" /> 调度一轮
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-8">
          <div className="bg-slate-800 rounded-2xl p-5 border border-slate-700">
            <div className="text-slate-400 text-sm flex items-center gap-1 mb-2"><Target className="w-4 h-4" />目标</div>
            <div className="text-white font-semibold">{study.objective_metric || '—'}</div>
            <div className="text-slate-500 text-xs mt-1">{study.objective_direction === 'minimize' ? '最小化' : '最大化'}</div>
          </div>
          <div className="bg-slate-800 rounded-2xl p-5 border border-slate-700">
            <div className="text-slate-400 text-sm flex items-center gap-1 mb-2"><GitBranch className="w-4 h-4" />试验</div>
            <div className="text-white font-semibold">{trials.length} / {study.max_trials}</div>
            <div className="text-slate-500 text-xs mt-1">并行 {study.max_parallel}</div>
          </div>
          <div className="bg-slate-800 rounded-2xl p-5 border border-slate-700">
            <div className="text-slate-400 text-sm flex items-center gap-1 mb-2"><Trophy className="w-4 h-4" />最优值</div>
            <div className="text-white font-semibold">
              {study.best_trial_id ? num(trials.find((t) => t.id === study.best_trial_id)?.value) : '—'}
            </div>
            <div className="text-slate-500 text-xs mt-1 font-mono truncate">{study.best_trial_id || ''}</div>
          </div>
          <div className="bg-slate-800 rounded-2xl p-5 border border-slate-700">
            <div className="text-slate-400 text-sm flex items-center gap-1 mb-2"><Activity className="w-4 h-4" />搜索空间</div>
            <div className="text-white font-semibold">{(() => { try { return JSON.parse(study.space_json || '{}').params?.length || 0 } catch { return 0 } })()}</div>
            <div className="text-slate-500 text-xs mt-1">个超参</div>
          </div>
        </div>

        <div className="bg-slate-800 rounded-2xl border border-slate-700 overflow-hidden">
          <div className="px-6 py-4 border-b border-slate-700">
            <h2 className="text-lg font-semibold text-white">试验列表</h2>
          </div>
          {trials.length === 0 ? (
            <div className="p-12 text-center text-slate-500">暂无试验，点击「调度一轮」拉起首个试验</div>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="text-slate-400 text-left border-b border-slate-700">
                  <th className="px-6 py-3">#</th>
                  <th className="px-6 py-3">状态</th>
                  <th className="px-6 py-3">参数</th>
                  <th className="px-6 py-3">目标值</th>
                  <th className="px-6 py-3">Job</th>
                </tr>
              </thead>
              <tbody>
                {trials.map((t) => (
                  <tr key={t.id} className="border-b border-slate-700/50 hover:bg-slate-700/30">
                    <td className="px-6 py-3 text-slate-300">{t.number}</td>
                    <td className="px-6 py-3"><span className={`text-xs px-2 py-0.5 rounded-full ${statusStyle(t.status)}`}>{t.status}</span></td>
                    <td className="px-6 py-3 text-slate-300 font-mono text-xs max-w-md truncate" title={t.params_json}>{t.params_json}</td>
                    <td className="px-6 py-3 text-slate-300">{num(t.value)}</td>
                    <td className="px-6 py-3 text-slate-500 font-mono text-xs">{t.job_id || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

export default AutoMLStudy
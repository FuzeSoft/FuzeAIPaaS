import React, { useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { apiFetch } from '../auth'
import { experimentStatusStyle } from '../utils/status'
import { Plus, FlaskConical, Trash2, Target, Archive, ArrowRight, GitCompareArrows } from 'lucide-react'

const parseTags = (raw) => {
  if (!raw) return []
  if (Array.isArray(raw)) return raw
  try {
    const v = JSON.parse(raw)
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

const Experiments = () => {
  const navigate = useNavigate()
  const [experiments, setExperiments] = useState([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [selected, setSelected] = useState([])
  const [newExp, setNewExp] = useState({ name: '', description: '', metric_name: '', objective: 'maximize', tags: '' })

  useEffect(() => { fetchExperiments() }, [])

  const fetchExperiments = async () => {
    try {
      const res = await apiFetch('/api/v1/experiments')
      const data = await res.json()
      setExperiments(Array.isArray(data) ? data : [])
    } catch (e) {
      console.error('Error fetching experiments:', e)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (e) => {
    e.preventDefault()
    
    const payload = {
      ...newExp,
      tags: newExp.tags.split(',').map((t) => t.trim()).filter(Boolean),
    }
    try {
      const res = await apiFetch('/api/v1/experiments', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (res.ok) {
        setShowModal(false)
        setNewExp({ name: '', description: '', metric_name: '', objective: 'maximize', tags: '' })
        fetchExperiments()
      } else {
        alert('创建实验失败')
      }
    } catch (e) {
      console.error('Error creating experiment:', e)
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除该实验及其全部运行记录？')) return
    try {
      await apiFetch(`/api/v1/experiments/${id}`, { method: 'DELETE' })
      fetchExperiments()
    } catch (e) {
      console.error('Error deleting experiment:', e)
    }
  }

  const handleArchive = async (id) => {
    try {
      const res = await apiFetch(`/api/v1/experiments/${id}/archive`, { method: 'POST' })
      if (res.ok) fetchExperiments()
      else alert('归档失败')
    } catch (e) {
      console.error('Error archiving experiment:', e)
    }
  }

  const toggleSelect = (id) => {
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  }

  if (loading) {
    return <div className="flex items-center justify-center min-h-screen"><div className="text-slate-400">加载中...</div></div>
  }

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2">实验管理</h1>
            <p className="text-slate-400">记录训练实验的运行、超参与指标，支撑复现与对比</p>
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={() => navigate(`/experiments/compare?ids=${selected.join(',')}`)}
              disabled={selected.length < 2}
              title={selected.length < 2 ? '至少选择 2 个实验进行对比' : '跨实验对比'}
              className={`flex items-center gap-2 px-5 py-3 rounded-xl font-medium transition-all ${
                selected.length >= 2
                  ? 'bg-slate-700 hover:bg-slate-600 text-blue-300'
                  : 'bg-slate-800 text-slate-600 cursor-not-allowed'
              }`}
            >
              <GitCompareArrows className="w-5 h-5" />
              对比选中（{selected.length}）
            </button>
            <button
              onClick={() => setShowModal(true)}
              className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl"
            >
              <Plus className="w-5 h-5" />
              创建实验
            </button>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {experiments.map((exp) => {
            const style = experimentStatusStyle(exp.status)
            const StatusIcon = style.icon
            const tags = parseTags(exp.tags)
            return (
              <div key={exp.id} className={`bg-slate-800 rounded-2xl border shadow-xl p-6 transition-all ${
                selected.includes(exp.id) ? 'border-blue-500/70 ring-2 ring-blue-500/30' : 'border-slate-700 hover:border-blue-500/50'
              }`}>
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3 min-w-0">
                    <button
                      onClick={() => toggleSelect(exp.id)}
                      title="加入对比"
                      className={`w-6 h-6 rounded-md border flex items-center justify-center shrink-0 transition-colors ${
                        selected.includes(exp.id)
                          ? 'bg-blue-600 border-blue-600 text-white'
                          : 'border-slate-500 text-transparent hover:border-blue-400'
                      }`}
                    >
                      ✓
                    </button>
                    <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-xl flex items-center justify-center shrink-0">
                      <FlaskConical className="w-5 h-5 text-white" />
                    </div>
                    <div className="min-w-0">
                      <h3 className="font-bold text-white truncate" title={exp.name}>{exp.name}</h3>
                      <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full ${style.bg} ${style.text}`}>
                        {StatusIcon && <StatusIcon className="w-3 h-3" />}
                        {style.label}
                      </span>
                    </div>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    {exp.status !== 'archived' && (
                      <button onClick={() => handleArchive(exp.id)} title="归档实验" className="p-2 text-slate-400 hover:text-yellow-400 hover:bg-slate-700 rounded-lg transition-colors">
                        <Archive className="w-4 h-4" />
                      </button>
                    )}
                    <button onClick={() => handleDelete(exp.id)} title="删除实验" className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg transition-colors">
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>

                <p className="text-sm text-slate-400 mb-3 line-clamp-2">{exp.description || '暂无描述'}</p>

                <div className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-slate-400 flex items-center gap-1"><Target className="w-3.5 h-3.5" />优化目标</span>
                    <span className="text-slate-300">
                      {exp.metric_name ? `${exp.objective === 'minimize' ? '最小化' : '最大化'} ${exp.metric_name}` : '—'}
                    </span>
                  </div>
                </div>

                {tags.length > 0 && (
                  <div className="flex flex-wrap gap-1.5 mt-3">
                    {tags.map((t) => (
                      <span key={t} className="text-[11px] px-2 py-0.5 rounded-full bg-slate-700 text-slate-300">{t}</span>
                    ))}
                  </div>
                )}

                <Link
                  to={`/experiments/${exp.id}`}
                  className="mt-4 w-full flex items-center justify-center gap-2 text-sm text-blue-400 hover:text-blue-300 py-2 rounded-lg border border-slate-700 hover:border-blue-500/50 transition-colors"
                >
                  查看运行记录
                  <ArrowRight className="w-4 h-4" />
                </Link>
              </div>
            )
          })}
          {experiments.length === 0 && (
            <div className="col-span-full text-center text-slate-500 py-16">暂无实验，点击「创建实验」开始</div>
          )}
        </div>

        {showModal && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-lg border border-slate-700 shadow-2xl">
              <h2 className="text-2xl font-bold text-white mb-6">创建实验</h2>
              <form onSubmit={handleCreate} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">实验名称</label>
                  <input type="text" value={newExp.name} onChange={(e) => setNewExp({ ...newExp, name: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" required />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">目标指标</label>
                    <input type="text" value={newExp.metric_name} onChange={(e) => setNewExp({ ...newExp, metric_name: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="如 val_accuracy" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">优化方向</label>
                    <select value={newExp.objective} onChange={(e) => setNewExp({ ...newExp, objective: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                      <option value="maximize">最大化</option>
                      <option value="minimize">最小化</option>
                    </select>
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">标签</label>
                  <input type="text" value={newExp.tags} onChange={(e) => setNewExp({ ...newExp, tags: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="以逗号分隔，如 nlp,baseline" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">描述</label>
                  <textarea value={newExp.description} onChange={(e) => setNewExp({ ...newExp, description: e.target.value })} rows={3} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="实验目标、假设等" />
                </div>
                <div className="flex gap-3 mt-6">
                  <button type="button" onClick={() => setShowModal(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">取消</button>
                  <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all">创建</button>
                </div>
              </form>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default Experiments
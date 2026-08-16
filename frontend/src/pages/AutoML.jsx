import React, { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { apiFetch } from '../auth'
import { Plus, Target, Trash2, Play, ChevronRight, Sparkles, GitBranch } from 'lucide-react'

const emptyParam = () => ({ name: '', type: 'float', min: '', max: '', step: '', log_scale: false, choices: '' })

const AutoML = () => {
  const [studies, setStudies] = useState([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [form, setForm] = useState({
    name: '',
    objective_metric: '',
    objective_direction: 'maximize',
    algorithm: 'tpe',
    max_trials: 20,
    max_parallel: 2,
    early_stop_enabled: false,
    early_stop_eta: 3,
    training_template: '',
  })
  const [params, setParams] = useState([emptyParam()])

  useEffect(() => { fetchStudies() }, [])

  const fetchStudies = async () => {
    try {
      const res = await apiFetch('/api/v1/hpo')
      const data = await res.json()
      setStudies(Array.isArray(data) ? data : [])
    } catch (e) {
      console.error('Error fetching studies:', e)
    } finally {
      setLoading(false)
    }
  }

  const updateParam = (i, patch) => {
    setParams((prev) => prev.map((p, idx) => (idx === i ? { ...p, ...patch } : p)))
  }

  const handleCreate = async (e) => {
    e.preventDefault()
    const search_space = params
      .filter((p) => p.name)
      .map((p) => {
        const out = { name: p.name, type: p.type, log_scale: !!p.log_scale }
        if (p.type === 'float' || p.type === 'int') {
          if (p.min !== '') out.min = Number(p.min)
          if (p.max !== '') out.max = Number(p.max)
          if (p.step !== '') out.step = Number(p.step)
        } else if (p.type === 'categorical' || p.type === 'bool') {
          out.choices = p.choices.split(',').map((c) => c.trim()).filter(Boolean)
        }
        return out
      })
    if (search_space.length === 0) {
      alert('至少添加一个搜索空间参数')
      return
    }
    const payload = {
      ...form,
      search_space,
    }
    if (!form.early_stop_enabled) {
      delete payload.early_stop_enabled
      delete payload.early_stop_eta
    } else {
      payload.early_stop = { enabled: true, eta: Number(form.early_stop_eta), min_rungs: 1 }
      delete payload.early_stop_enabled
      delete payload.early_stop_eta
    }
    if (form.training_template) {
      try {
        payload.training_template = JSON.parse(form.training_template)
      } catch {
        alert('训练模板需为合法 JSON')
        return
      }
    } else {
      delete payload.training_template
    }
    try {
      const res = await apiFetch('/api/v1/hpo', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (res.ok) {
        setShowModal(false)
        setForm({ name: '', objective_metric: '', objective_direction: 'maximize', algorithm: 'tpe', max_trials: 20, max_parallel: 2, early_stop_enabled: false, early_stop_eta: 3, training_template: '' })
        setParams([emptyParam()])
        fetchStudies()
      } else {
        const d = await res.json().catch(() => ({}))
        alert('创建失败：' + (d.error || res.statusText))
      }
    } catch (e) {
      console.error('Error creating study:', e)
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除该研究及其全部试验？')) return
    try {
      await apiFetch(`/api/v1/hpo/${id}`, { method: 'DELETE' })
      fetchStudies()
    } catch (e) {
      console.error('Error deleting study:', e)
    }
  }

  const handleRun = async (id) => {
    try {
      await apiFetch(`/api/v1/hpo/${id}/run`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })
      fetchStudies()
    } catch (e) {
      console.error('Error running study:', e)
    }
  }

  if (loading) {
    return <div className="flex items-center justify-center min-h-screen"><div className="text-slate-400">加载中...</div></div>
  }

  const statusStyle = (s) => {
    if (s === 'completed') return { c: 'text-green-300 bg-green-500/10', t: '已完成' }
    if (s === 'running') return { c: 'text-blue-300 bg-blue-500/10', t: '运行中' }
    if (s === 'failed') return { c: 'text-red-300 bg-red-500/10', t: '失败' }
    return { c: 'text-slate-300 bg-slate-500/10', t: '待运行' }
  }

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2 flex items-center gap-2">
              <Sparkles className="w-7 h-7 text-purple-400" /> AutoML / NAS
            </h1>
            <p className="text-slate-400">自动超参搜索与神经架构搜索：搜索空间 → 采样 → 训练 → 早停 → 收敛最优</p>
          </div>
          <button
            onClick={() => setShowModal(true)}
            className="flex items-center gap-2 bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl"
          >
            <Plus className="w-5 h-5" /> 创建研究
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {studies.map((s) => {
            const st = statusStyle(s.status)
            return (
              <div key={s.id} className="bg-slate-800 rounded-2xl border border-slate-700 hover:border-purple-500/50 shadow-xl p-6 transition-all">
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="w-10 h-10 bg-gradient-to-br from-purple-500 to-blue-600 rounded-xl flex items-center justify-center shrink-0">
                      <GitBranch className="w-5 h-5 text-white" />
                    </div>
                    <div className="min-w-0">
                      <h3 className="font-bold text-white truncate" title={s.name}>{s.name}</h3>
                      <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full ${st.c}`}>{st.t}</span>
                    </div>
                  </div>
                  <button onClick={() => handleDelete(s.id)} title="删除研究" className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg transition-colors">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>

                <div className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-slate-400 flex items-center gap-1"><Target className="w-3.5 h-3.5" />目标</span>
                    <span className="text-slate-300">{s.objective_metric ? `${s.objective_direction === 'minimize' ? '最小化' : '最大化'} ${s.objective_metric}` : '—'}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-slate-400">算法</span>
                    <span className="text-slate-300">{s.algorithm || 'tpe'}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-slate-400">试验上限</span>
                    <span className="text-slate-300">{s.max_trials}（并行 {s.max_parallel}）</span>
                  </div>
                  {s.best_trial_id && (
                    <div className="flex justify-between">
                      <span className="text-slate-400">最优试验</span>
                      <span className="text-green-300 font-mono text-xs">{s.best_trial_id}</span>
                    </div>
                  )}
                </div>

                <div className="mt-4 flex items-center gap-2">
                  <button
                    onClick={() => handleRun(s.id)}
                    className="flex-1 flex items-center justify-center gap-2 text-sm text-blue-300 hover:text-blue-200 py-2 rounded-lg border border-slate-700 hover:border-blue-500/50 transition-colors"
                  >
                    <Play className="w-4 h-4" /> 调度一轮
                  </button>
                  <Link
                    to={`/automl/${s.id}`}
                    className="flex-1 flex items-center justify-center gap-2 text-sm text-purple-300 hover:text-purple-200 py-2 rounded-lg border border-slate-700 hover:border-purple-500/50 transition-colors"
                  >
                    查看试验 <ChevronRight className="w-4 h-4" />
                  </Link>
                </div>
              </div>
            )
          })}
          {studies.length === 0 && (
            <div className="col-span-full text-center text-slate-500 py-16">暂无研究，点击「创建研究」开始自动调参</div>
          )}
        </div>

        {showModal && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4 overflow-y-auto">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-2xl border border-slate-700 shadow-2xl my-8">
              <h2 className="text-2xl font-bold text-white mb-6">创建 AutoML 研究</h2>
              <form onSubmit={handleCreate} className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">研究名称</label>
                    <input type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-purple-500" required />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">算法</label>
                    <select value={form.algorithm} onChange={(e) => setForm({ ...form, algorithm: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-purple-500">
                      <option value="tpe">TPE（贝叶斯）</option>
                      <option value="random">Random（随机）</option>
                      <option value="grid">Grid（网格）</option>
                    </select>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">目标指标</label>
                    <input type="text" value={form.objective_metric} onChange={(e) => setForm({ ...form, objective_metric: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-purple-500" placeholder="如 val_accuracy" required />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">优化方向</label>
                    <select value={form.objective_direction} onChange={(e) => setForm({ ...form, objective_direction: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-purple-500">
                      <option value="maximize">最大化</option>
                      <option value="minimize">最小化</option>
                    </select>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">试验上限</label>
                    <input type="number" value={form.max_trials} onChange={(e) => setForm({ ...form, max_trials: Number(e.target.value) })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-purple-500" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">并行度</label>
                    <input type="number" value={form.max_parallel} onChange={(e) => setForm({ ...form, max_parallel: Number(e.target.value) })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-purple-500" />
                  </div>
                </div>

                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">搜索空间</label>
                  <div className="space-y-2">
                    {params.map((p, i) => (
                      <div key={i} className="flex items-center gap-2">
                        <input type="text" value={p.name} onChange={(e) => updateParam(i, { name: e.target.value })} placeholder="参数名" className="flex-1 bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-purple-500" />
                        <select value={p.type} onChange={(e) => updateParam(i, { type: e.target.value })} className="bg-slate-700 border border-slate-600 rounded-lg px-2 py-2 text-white text-sm focus:outline-none focus:border-purple-500">
                          <option value="float">float</option>
                          <option value="int">int</option>
                          <option value="categorical">categorical</option>
                          <option value="bool">bool</option>
                        </select>
                        {(p.type === 'float' || p.type === 'int') ? (
                          <>
                            <input type="number" value={p.min} onChange={(e) => updateParam(i, { min: e.target.value })} placeholder="min" className="w-16 bg-slate-700 border border-slate-600 rounded-lg px-2 py-2 text-white text-sm focus:outline-none focus:border-purple-500" />
                            <input type="number" value={p.max} onChange={(e) => updateParam(i, { max: e.target.value })} placeholder="max" className="w-16 bg-slate-700 border border-slate-600 rounded-lg px-2 py-2 text-white text-sm focus:outline-none focus:border-purple-500" />
                            <input type="number" value={p.step} onChange={(e) => updateParam(i, { step: e.target.value })} placeholder="step" className="w-14 bg-slate-700 border border-slate-600 rounded-lg px-2 py-2 text-white text-sm focus:outline-none focus:border-purple-500" />
                            <label className="flex items-center gap-1 text-xs text-slate-400"><input type="checkbox" checked={p.log_scale} onChange={(e) => updateParam(i, { log_scale: e.target.checked })} />log</label>
                          </>
                        ) : (
                          <input type="text" value={p.choices} onChange={(e) => updateParam(i, { choices: e.target.value })} placeholder="候选(逗号分隔)" className="flex-1 bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-purple-500" />
                        )}
                        <button type="button" onClick={() => setParams((prev) => prev.filter((_, idx) => idx !== i))} className="text-slate-400 hover:text-red-400 px-2">✕</button>
                      </div>
                    ))}
                    <button type="button" onClick={() => setParams((prev) => [...prev, emptyParam()])} className="text-sm text-purple-300 hover:text-purple-200">+ 添加参数</button>
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  <input type="checkbox" checked={form.early_stop_enabled} onChange={(e) => setForm({ ...form, early_stop_enabled: e.target.checked })} />
                  <label className="text-sm text-slate-300">启用 ASHA 早停（eta=</label>
                  <input type="number" value={form.early_stop_eta} onChange={(e) => setForm({ ...form, early_stop_eta: Number(e.target.value) })} className="w-14 bg-slate-700 border border-slate-600 rounded-lg px-2 py-1 text-white text-sm" />
                  <label className="text-sm text-slate-300">）</label>
                </div>

                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">训练模板 JSON（可选）</label>
                  <textarea value={form.training_template} onChange={(e) => setForm({ ...form, training_template: e.target.value })} rows={2} placeholder='{"image":"train:latest","command":"python train.py"}' className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white text-sm focus:outline-none focus:border-purple-500 font-mono" />
                </div>

                <div className="flex gap-3 mt-6">
                  <button type="button" onClick={() => setShowModal(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">取消</button>
                  <button type="submit" className="flex-1 bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 text-white px-6 py-3 rounded-xl font-medium transition-all">创建</button>
                </div>
              </form>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default AutoML
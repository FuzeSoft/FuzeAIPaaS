import React, { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { apiFetch } from '../auth'
import { runStatusStyle, judgeModeStyle } from '../utils/status'
import { Plus, ClipboardCheck, Trash2, ArrowRight, FlaskConical, Boxes } from 'lucide-react'

const Evaluations = () => {
  const [evals, setEvals] = useState([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [filter, setFilter] = useState('all')
  const [newEval, setNewEval] = useState({
    name: '',
    task: '',
    dataset: '',
    model_id: '',
    experiment_id: '',
    run_id: '',
    judge_mode: 'threshold',
    criteria: '{"accuracy": {"op": ">=", "value": 0.8}}',
    dimensions: '[{"name":"accuracy","weight":0.5,"description":"答案正确性"},{"name":"fluency","weight":0.3,"description":"表达流畅度"},{"name":"relevance","weight":0.2,"description":"相关性"}]',
  })

  useEffect(() => {
    fetchEvals()
  }, [])

  const fetchEvals = async () => {
    try {
      const res = await apiFetch('/api/v1/evaluations')
      const data = await res.json()
      setEvals(Array.isArray(data.evaluations) ? data.evaluations : [])
    } catch (e) {
      console.error('Error fetching evaluations:', e)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (e) => {
    e.preventDefault()
    const payload = { ...newEval }
    
    if (payload.judge_mode !== 'threshold') {
      delete payload.criteria
    } else {
      delete payload.dimensions
    }
    try {
      const res = await apiFetch('/api/v1/evaluations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (res.ok) {
        setShowModal(false)
        fetchEvals()
      } else {
        const err = await res.json().catch(() => ({}))
        alert('创建评估失败：' + (err.error || res.statusText))
      }
    } catch (e) {
      console.error('Error creating evaluation:', e)
      alert('创建评估失败：网络错误')
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除该评估及其全部评审记录？')) return
    try {
      await apiFetch(`/api/v1/evaluations/${id}`, { method: 'DELETE' })
      fetchEvals()
    } catch (e) {
      console.error('Error deleting evaluation:', e)
    }
  }

  const filtered = evals.filter((ev) => {
    if (filter === 'all') return true
    const mode = ev.judge_mode || 'threshold'
    return mode === filter
  })

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-slate-400">加载中...</div>
      </div>
    )
  }

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2">评估管理</h1>
            <p className="text-slate-400">支持阈值裁决、人工评审、LLM-as-judge 与混合评审多维度评估</p>
          </div>
          <button
            onClick={() => setShowModal(true)}
            className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl"
          >
            <Plus className="w-5 h-5" />
            创建评估
          </button>
        </div>

        {}
        <div className="flex flex-wrap gap-2 mb-6">
          {[
            { key: 'all', label: '全部' },
            { key: 'threshold', label: '阈值裁决' },
            { key: 'human', label: '人工评审' },
            { key: 'llm', label: 'LLM 评审' },
            { key: 'hybrid', label: '混合评审' },
          ].map((f) => (
            <button
              key={f.key}
              onClick={() => setFilter(f.key)}
              className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                filter === f.key
                  ? 'bg-gradient-to-r from-blue-600 to-purple-600 text-white shadow-lg'
                  : 'bg-slate-800 text-slate-400 hover:bg-slate-700 hover:text-white'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {filtered.map((ev) => {
            const style = runStatusStyle(ev.status)
            const StatusIcon = style.icon
            const modeStyle = judgeModeStyle(ev.judge_mode)
            return (
              <div
                key={ev.id}
                className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6 hover:border-blue-500/50 transition-all"
              >
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="w-10 h-10 bg-gradient-to-br from-purple-500 to-pink-600 rounded-xl flex items-center justify-center shrink-0">
                      <ClipboardCheck className="w-5 h-5 text-white" />
                    </div>
                    <div className="min-w-0">
                      <h3 className="font-bold text-white truncate" title={ev.name}>{ev.name}</h3>
                      <div className="flex items-center gap-2 mt-1">
                        <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full ${style.bg} ${style.text}`}>
                          {StatusIcon && <StatusIcon className="w-3 h-3" />}
                          {style.label}
                        </span>
                        <span className={`inline-flex items-center text-xs px-2 py-0.5 rounded-full ${modeStyle.bg} ${modeStyle.text}`}>
                          {modeStyle.label}
                        </span>
                      </div>
                    </div>
                  </div>
                  <button
                    onClick={() => handleDelete(ev.id)}
                    title="删除评估"
                    className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg transition-colors shrink-0"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>

                <div className="space-y-2 text-sm">
                  {ev.task && (
                    <div className="flex justify-between">
                      <span className="text-slate-400">任务</span>
                      <span className="text-slate-300 truncate ml-2" title={ev.task}>{ev.task}</span>
                    </div>
                  )}
                  {ev.model_id && (
                    <div className="flex justify-between items-center">
                      <span className="text-slate-400 flex items-center gap-1"><Boxes className="w-3.5 h-3.5" />模型</span>
                      <span className="text-slate-300 truncate ml-2" title={ev.model_id}>{ev.model_id}</span>
                    </div>
                  )}
                  {ev.experiment_id && (
                    <div className="flex justify-between items-center">
                      <span className="text-slate-400 flex items-center gap-1"><FlaskConical className="w-3.5 h-3.5" />实验</span>
                      <span className="text-slate-300 truncate ml-2" title={ev.experiment_id}>{ev.experiment_id}</span>
                    </div>
                  )}
                  {ev.status === 'completed' && (
                    <div className="flex justify-between">
                      <span className="text-slate-400">综合得分</span>
                      <span className={`font-bold ${ev.passed ? 'text-green-400' : 'text-red-400'}`}>
                        {(ev.score * 100).toFixed(1)}分
                      </span>
                    </div>
                  )}
                </div>

                <Link
                  to={`/evaluations/${ev.id}`}
                  className="mt-4 w-full flex items-center justify-center gap-2 text-sm text-blue-400 hover:text-blue-300 py-2 rounded-lg border border-slate-700 hover:border-blue-500/50 transition-colors"
                >
                  查看评估报告
                  <ArrowRight className="w-4 h-4" />
                </Link>
              </div>
            )
          })}
          {filtered.length === 0 && (
            <div className="col-span-full text-center text-slate-500 py-16">暂无评估，点击「创建评估」开始</div>
          )}
        </div>

        {showModal && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-2xl border border-slate-700 shadow-2xl max-h-[90vh] overflow-y-auto">
              <h2 className="text-2xl font-bold text-white mb-6">创建评估</h2>
              <form onSubmit={handleCreate} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">评估名称</label>
                  <input
                    type="text"
                    value={newEval.name}
                    onChange={(e) => setNewEval({ ...newEval, name: e.target.value })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    required
                  />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">任务描述</label>
                    <input
                      type="text"
                      value={newEval.task}
                      onChange={(e) => setNewEval({ ...newEval, task: e.target.value })}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                      placeholder="如 text-classification"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">数据集</label>
                    <input
                      type="text"
                      value={newEval.dataset}
                      onChange={(e) => setNewEval({ ...newEval, dataset: e.target.value })}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    />
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">评估模式</label>
                  <select
                    value={newEval.judge_mode}
                    onChange={(e) => setNewEval({ ...newEval, judge_mode: e.target.value })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                  >
                    <option value="threshold">阈值裁决（自动指标对比）</option>
                    <option value="human">人工评审（多维度打分）</option>
                    <option value="llm">LLM 评审（LLM-as-judge 自动打分）</option>
                    <option value="hybrid">混合评审（人工 + LLM）</option>
                  </select>
                </div>
                {newEval.judge_mode === 'threshold' ? (
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">
                      准则 JSON（阈值裁决）
                    </label>
                    <textarea
                      value={newEval.criteria}
                      onChange={(e) => setNewEval({ ...newEval, criteria: e.target.value })}
                      rows={3}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500 font-mono text-sm"
                    />
                    <p className="text-xs text-slate-500 mt-1">格式：{"{metric: {op: '>=', value: 0.8}}"}</p>
                  </div>
                ) : (
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">
                      维度定义 JSON（{judgeModeStyle(newEval.judge_mode).label}）
                    </label>
                    <textarea
                      value={newEval.dimensions}
                      onChange={(e) => setNewEval({ ...newEval, dimensions: e.target.value })}
                      rows={5}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500 font-mono text-sm"
                    />
                    <p className="text-xs text-slate-500 mt-1">
                      格式：[{"{name, weight, description}"}]，权重和无需为 1（后端自动归一化）
                    </p>
                  </div>
                )}
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">关联模型 ID（可选）</label>
                    <input
                      type="text"
                      value={newEval.model_id}
                      onChange={(e) => setNewEval({ ...newEval, model_id: e.target.value })}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                      placeholder="model-xxx"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">关联实验 ID（可选）</label>
                    <input
                      type="text"
                      value={newEval.experiment_id}
                      onChange={(e) => setNewEval({ ...newEval, experiment_id: e.target.value })}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                      placeholder="exp-xxx（需同时填 Run ID）"
                    />
                  </div>
                </div>
                {newEval.experiment_id && (
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">运行 ID</label>
                    <input
                      type="text"
                      value={newEval.run_id}
                      onChange={(e) => setNewEval({ ...newEval, run_id: e.target.value })}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                      placeholder="run-xxx"
                    />
                  </div>
                )}
                <div className="flex gap-3 mt-6">
                  <button
                    type="button"
                    onClick={() => setShowModal(false)}
                    className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors"
                  >
                    取消
                  </button>
                  <button
                    type="submit"
                    className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all"
                  >
                    创建
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default Evaluations
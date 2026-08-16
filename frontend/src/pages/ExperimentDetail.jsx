import React, { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts'
import { apiFetch } from '../auth'
import { runStatusStyle, experimentStatusStyle, reproductionStateStyle } from '../utils/status'
import { ArrowLeft, Plus, Trophy, FlaskConical, Target, BarChart3, GitBranch, FileSearch } from 'lucide-react'

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

const ExperimentDetail = () => {
  const { id } = useParams()
  const [experiment, setExperiment] = useState(null)
  const [runs, setRuns] = useState([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [newRun, setNewRun] = useState({ name: '', hyperparameters: '', source_job_id: '' })
  const [formError, setFormError] = useState('')
  const [reproBusy, setReproBusy] = useState('')
  const [report, setReport] = useState(null)

  useEffect(() => { fetchAll() }, [id])

  const fetchAll = async () => {
    try {
      const [expRes, runsRes] = await Promise.all([
        apiFetch(`/api/v1/experiments/${id}`),
        apiFetch(`/api/v1/experiments/${id}/runs`),
      ])
      const expData = await expRes.json()
      const runsData = await runsRes.json()
      setExperiment(expData && expData.id ? expData : null)
      setRuns(Array.isArray(runsData) ? runsData : [])
    } catch (e) {
      console.error('Error fetching experiment detail:', e)
    } finally {
      setLoading(false)
    }
  }

  const handleCreateRun = async (e) => {
    e.preventDefault()
    setFormError('')
    let hyper = {}
    if (newRun.hyperparameters.trim()) {
      try {
        hyper = JSON.parse(newRun.hyperparameters)
      } catch {
        
        setFormError('超参必须是合法的 JSON 对象，例如 {"lr": 0.001}')
        return
      }
    }
    try {
      const res = await apiFetch(`/api/v1/experiments/${id}/runs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newRun.name, hyperparameters: hyper, source_job_id: newRun.source_job_id }),
      })
      if (res.ok) {
        setShowModal(false)
        setNewRun({ name: '', hyperparameters: '', source_job_id: '' })
        fetchAll()
      } else {
        setFormError('创建运行失败')
      }
    } catch (err) {
      console.error('Error creating run:', err)
      setFormError('创建运行失败：' + err.message)
    }
  }

  const handleReproduce = async (runId) => {
    setReproBusy(runId)
    try {
      const res = await apiFetch(`/api/v1/experiments/runs/${runId}/reproduce`, { method: 'POST' })
      if (res.ok) {
        fetchAll()
      } else {
        alert('复现提交失败')
      }
    } catch (err) {
      console.error('Error reproducing run:', err)
      alert('复现提交失败：' + err.message)
    } finally {
      setReproBusy('')
    }
  }

  const openReport = async (runId) => {
    try {
      const res = await apiFetch(`/api/v1/experiments/runs/${runId}/reproduction`)
      const data = await res.json()
      setReport(data)
    } catch (err) {
      console.error('Error fetching reproduction report:', err)
      alert('获取复现报告失败')
    }
  }

  if (loading) {
    return <div className="flex items-center justify-center min-h-screen"><div className="text-slate-400">加载中...</div></div>
  }

  if (!experiment) {
    return (
      <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
        <div className="max-w-7xl mx-auto">
          <Link to="/experiments" className="inline-flex items-center gap-2 text-slate-400 hover:text-white mb-6">
            <ArrowLeft className="w-4 h-4" />返回实验列表
          </Link>
          <div className="text-center text-slate-500 py-16">实验不存在或已被删除</div>
        </div>
      </div>
    )
  }

  const expStyle = experimentStatusStyle(experiment.status)
  
  const chartData = runs
    .filter((r) => typeof r.metric_value === 'number')
    .map((r, i) => ({ name: r.name || `run-${i + 1}`, value: fmt(r.metric_value) }))

  const metricKeys = Array.from(
    new Set(runs.flatMap((r) => Object.keys(parseJSON(r.metrics)))),
  ).slice(0, 6)

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <Link to="/experiments" className="inline-flex items-center gap-2 text-slate-400 hover:text-white mb-6">
          <ArrowLeft className="w-4 h-4" />返回实验列表
        </Link>

        <div className="flex items-start justify-between mb-8">
          <div className="min-w-0">
            <div className="flex items-center gap-3 mb-2">
              <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-xl flex items-center justify-center shrink-0">
                <FlaskConical className="w-5 h-5 text-white" />
              </div>
              <h1 className="text-3xl font-bold text-white truncate">{experiment.name}</h1>
              <span className={`text-xs px-2 py-1 rounded-full ${expStyle.bg} ${expStyle.text}`}>{expStyle.label}</span>
            </div>
            <p className="text-slate-400">{experiment.description || '暂无描述'}</p>
          </div>
          <button
            onClick={() => setShowModal(true)}
            className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg shrink-0"
          >
            <Plus className="w-5 h-5" />
            新建运行
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
          <div className="bg-slate-800 rounded-2xl border border-slate-700 p-5">
            <div className="flex items-center gap-2 text-slate-400 text-sm mb-1"><Target className="w-4 h-4" />优化目标</div>
            <div className="text-white font-semibold">
              {experiment.metric_name ? `${experiment.objective === 'minimize' ? '最小化' : '最大化'} ${experiment.metric_name}` : '未设置'}
            </div>
          </div>
          <div className="bg-slate-800 rounded-2xl border border-slate-700 p-5">
            <div className="flex items-center gap-2 text-slate-400 text-sm mb-1"><BarChart3 className="w-4 h-4" />运行数</div>
            <div className="text-white font-semibold">{runs.length}</div>
          </div>
          <div className="bg-slate-800 rounded-2xl border border-slate-700 p-5">
            <div className="flex items-center gap-2 text-slate-400 text-sm mb-1"><Trophy className="w-4 h-4" />最优运行</div>
            <div className="text-white font-semibold truncate">
              {runs.find((r) => r.id === experiment.best_run_id)?.name || '—'}
            </div>
          </div>
        </div>

        <div className="bg-slate-800 rounded-2xl border border-slate-700 p-6 mb-8">
          <h2 className="text-lg font-bold text-white mb-4">
            指标趋势{experiment.metric_name ? `（${experiment.metric_name}）` : ''}
          </h2>
          {chartData.length > 0 ? (
            <ResponsiveContainer width="100%" height={280}>
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
                <XAxis dataKey="name" stroke="#94a3b8" fontSize={12} />
                <YAxis stroke="#94a3b8" fontSize={12} domain={['auto', 'auto']} />
                <Tooltip contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #334155', borderRadius: '0.75rem', color: '#fff' }} />
                <Legend />
                <Line type="monotone" dataKey="value" name={experiment.metric_name || '指标'} stroke="#3b82f6" strokeWidth={2} dot={{ r: 3 }} />
              </LineChart>
            </ResponsiveContainer>
          ) : (
            <div className="text-center text-slate-500 py-12">暂无已完成运行的指标数据</div>
          )}
        </div>

        <div className="bg-slate-800 rounded-2xl border border-slate-700 overflow-hidden">
          <h2 className="text-lg font-bold text-white p-6 pb-4">运行记录</h2>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-slate-900/60 text-slate-400">
                <tr>
                  <th className="text-left font-medium px-6 py-3">名称</th>
                  <th className="text-left font-medium px-6 py-3">状态</th>
                  <th className="text-left font-medium px-6 py-3">目标值</th>
                  {metricKeys.map((k) => (
                    <th key={k} className="text-left font-medium px-6 py-3">{k}</th>
                  ))}
                  <th className="text-left font-medium px-6 py-3">超参</th>
                  <th className="text-left font-medium px-6 py-3">复现闭环</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => {
                  const style = runStatusStyle(r.status)
                  const StatusIcon = style.icon
                  const metrics = parseJSON(r.metrics)
                  const hyper = parseJSON(r.hyperparameters)
                  const hyperText = Object.entries(hyper).map(([k, v]) => `${k}=${v}`).join(', ')
                  return (
                    <tr key={r.id} className="border-t border-slate-700/60 hover:bg-slate-700/20">
                      <td className="px-6 py-3">
                        <div className="flex items-center gap-2">
                          {r.id === experiment.best_run_id && <Trophy className="w-3.5 h-3.5 text-yellow-400" title="最优运行" />}
                          <span className="text-white">{r.name || r.id}</span>
                        </div>
                      </td>
                      <td className="px-6 py-3">
                        <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full ${style.bg} ${style.text}`}>
                          {StatusIcon && <StatusIcon className="w-3 h-3" />}
                          {style.label}
                        </span>
                      </td>
                      <td className="px-6 py-3 text-slate-300">
                        {typeof r.metric_value === 'number' ? fmt(r.metric_value) : '—'}
                      </td>
                      {metricKeys.map((k) => (
                        <td key={k} className="px-6 py-3 text-slate-300">
                          {typeof metrics[k] === 'number' ? fmt(metrics[k]) : (metrics[k] ?? '—')}
                        </td>
                      ))}
                      <td className="px-6 py-3 text-slate-400 max-w-xs truncate" title={hyperText}>{hyperText || '—'}</td>
                      <td className="px-6 py-3">
                        <div className="flex items-center gap-2 flex-wrap">
                          {r.parent_run_id && (() => {
                            const rp = reproductionStateStyle(r.reproduction_state)
                            if (!rp) return null
                            const RpIcon = rp.icon
                            return (
                              <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full ${rp.bg} ${rp.text}`}>
                                {RpIcon && <RpIcon className="w-3 h-3" />}
                                {rp.label}
                              </span>
                            )
                          })()}
                          {r.source_job_id && !r.parent_run_id && (
                            <button
                              onClick={() => handleReproduce(r.id)}
                              disabled={reproBusy === r.id}
                              title="克隆超参并复用训练任务触发复现"
                              className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded-lg bg-slate-700 hover:bg-slate-600 text-blue-300 transition-colors disabled:opacity-50"
                            >
                              {reproBusy === r.id ? '复现中…' : (<><GitBranch className="w-3 h-3" />复现</>)}
                            </button>
                          )}
                          {r.parent_run_id && (
                            <button
                              onClick={() => openReport(r.id)}
                              title="查看复现报告"
                              className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded-lg bg-slate-700 hover:bg-slate-600 text-purple-300 transition-colors"
                            >
                              <FileSearch className="w-3 h-3" />报告
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })}
                {runs.length === 0 && (
                  <tr>
                    <td colSpan={4 + metricKeys.length} className="text-center text-slate-500 py-12">
                      暂无运行记录，点击「新建运行」开始
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>

        {showModal && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-lg border border-slate-700 shadow-2xl">
              <h2 className="text-2xl font-bold text-white mb-6">新建运行</h2>
              <form onSubmit={handleCreateRun} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">运行名称</label>
                  <input type="text" value={newRun.name} onChange={(e) => setNewRun({ ...newRun, name: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="如 lr-0.001-bs-32" required />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">超参 (JSON)</label>
                  <textarea value={newRun.hyperparameters} onChange={(e) => setNewRun({ ...newRun, hyperparameters: e.target.value })} rows={4} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white font-mono text-sm focus:outline-none focus:border-blue-500" placeholder='{"lr": 0.001, "batch_size": 32}' />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">关联训练任务 ID (可选)</label>
                  <input type="text" value={newRun.source_job_id} onChange={(e) => setNewRun({ ...newRun, source_job_id: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="用于建立实验与训练执行的血缘" />
                </div>
                {formError && <div className="text-sm text-red-400">{formError}</div>}
                <div className="flex gap-3 mt-6">
                  <button type="button" onClick={() => { setShowModal(false); setFormError('') }} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">取消</button>
                  <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all">创建</button>
                </div>
              </form>
            </div>
          </div>
        )}

        {report && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-lg border border-slate-700 shadow-2xl">
              <div className="flex items-center justify-between mb-6">
                <h2 className="text-2xl font-bold text-white">复现报告</h2>
                <button onClick={() => setReport(null)} className="text-slate-400 hover:text-white text-2xl leading-none">×</button>
              </div>
              {report.status === 'pending' ? (
                <div className="text-slate-300">复现训练尚未完成，暂无指标可对比。</div>
              ) : (
                <>
                  <div className={`mb-5 inline-flex items-center gap-2 text-sm px-3 py-1.5 rounded-full ${
                    report.reproducible ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'
                  }`}>
                    {report.reproducible ? '✓ 可复现' : '✗ 偏差过大'}
                  </div>
                  <div className="space-y-3 text-sm">
                    <div className="flex justify-between"><span className="text-slate-400">目标指标</span><span className="text-slate-200">{report.metric_name}</span></div>
                    <div className="flex justify-between"><span className="text-slate-400">源 Run 指标</span><span className="text-slate-200">{fmt(report.source_metric)}</span></div>
                    <div className="flex justify-between"><span className="text-slate-400">复现 Run 指标</span><span className="text-slate-200">{fmt(report.repro_metric)}</span></div>
                    <div className="flex justify-between"><span className="text-slate-400">绝对偏差</span><span className="text-slate-200">{fmt(report.abs_deviation)}（阈值 {report.abs_tol}）</span></div>
                    <div className="flex justify-between"><span className="text-slate-400">相对偏差</span><span className="text-slate-200">{fmt(report.rel_deviation)}（阈值 {report.rel_tol}）</span></div>
                    <div className="flex justify-between"><span className="text-slate-400">判定口径</span><span className="text-slate-200">绝对或相对满足其一</span></div>
                  </div>
                </>
              )}
              <div className="flex mt-6">
                <button onClick={() => setReport(null)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">关闭</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default ExperimentDetail
import React, { useState, useEffect, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import { apiFetch } from '../auth'
import { runStatusStyle, judgeModeStyle, verdictStyle } from '../utils/status'
import {
  Radar,
  RadarChart,
  PolarGrid,
  PolarAngleAxis,
  PolarRadiusAxis,
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
} from 'recharts'
import {
  ArrowLeft,
  ClipboardCheck,
  User,
  Bot,
  Sparkles,
  CheckCircle,
  Play,
  RefreshCw,
} from 'lucide-react'

const EvaluationReport = () => {
  const { id } = useParams()
  const [evaluation, setEvaluation] = useState(null)
  const [report, setReport] = useState(null)
  const [reviews, setReviews] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [actionLoading, setActionLoading] = useState(false)
  const [showReviewForm, setShowReviewForm] = useState(false)
  const [reviewForm, setReviewForm] = useState({ scores: '', comment: '' })

  const fetchData = useCallback(async () => {
    try {
      const [evalRes, reportRes, reviewsRes] = await Promise.all([
        apiFetch(`/api/v1/evaluations/${id}`),
        apiFetch(`/api/v1/evaluations/${id}/report`),
        apiFetch(`/api/v1/evaluations/${id}/reviews`),
      ])
      if (evalRes.ok) setEvaluation(await evalRes.json())
      if (reportRes.ok) setReport(await reportRes.json())
      if (reviewsRes.ok) {
        const data = await reviewsRes.json()
        setReviews(Array.isArray(data.reviews) ? data.reviews : [])
      }
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }, [id])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const runLLMJudge = async () => {
    setActionLoading(true)
    try {
      const res = await apiFetch(`/api/v1/evaluations/${id}/llm-judge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        alert('LLM 评审失败：' + (err.error || res.statusText))
      } else {
        await fetchData()
      }
    } catch (e) {
      alert('LLM 评审失败：网络错误')
    } finally {
      setActionLoading(false)
    }
  }

  const submitReview = async (e) => {
    e.preventDefault()
    setActionLoading(true)
    try {
      const res = await apiFetch(`/api/v1/evaluations/${id}/reviews`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ scores: reviewForm.scores, comment: reviewForm.comment }),
      })
      if (res.ok) {
        setShowReviewForm(false)
        setReviewForm({ scores: '', comment: '' })
        await fetchData()
      } else {
        const err = await res.json().catch(() => ({}))
        alert('提交评审失败：' + (err.error || res.statusText))
      }
    } catch (e) {
      alert('提交评审失败：网络错误')
    } finally {
      setActionLoading(false)
    }
  }

  const finalize = async () => {
    if (!confirm('确定完成评估？将聚合全部评审生成最终报告，完成后不可再追加评审。')) return
    setActionLoading(true)
    try {
      const res = await apiFetch(`/api/v1/evaluations/${id}/finalize`, { method: 'POST' })
      if (res.ok) {
        await fetchData()
      } else {
        const err = await res.json().catch(() => ({}))
        alert('完成评估失败：' + (err.error || res.statusText))
      }
    } catch (e) {
      alert('完成评估失败：网络错误')
    } finally {
      setActionLoading(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-slate-400">加载中...</div>
      </div>
    )
  }

  if (error || !evaluation) {
    return (
      <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
        <div className="max-w-4xl mx-auto">
          <Link to="/evaluations" className="flex items-center gap-2 text-blue-400 hover:text-blue-300 mb-4">
            <ArrowLeft className="w-4 h-4" /> 返回评估列表
          </Link>
          <div className="text-red-400">{error || '评估不存在'}</div>
        </div>
      </div>
    )
  }

  const statusStyle = runStatusStyle(evaluation.status)
  const StatusIcon = statusStyle.icon
  const modeStyle = judgeModeStyle(evaluation.judge_mode)
  const vStyle = verdictStyle(report?.verdict)
  const VerdictIcon = vStyle.icon
  const isJudgeMode = ['human', 'llm', 'hybrid'].includes(evaluation.judge_mode || 'threshold')
  const isFinalized = evaluation.status === 'completed' || evaluation.status === 'failed'
  const dims = report?.dimensions || []
  const byJudgeType = report?.by_judge_type || {}

  const initReviewForm = () => {
    if (dims.length > 0) {
      const template = {}
      dims.forEach((d) => (template[d.name] = 0.8))
      setReviewForm({ scores: JSON.stringify(template, null, 2), comment: '' })
    }
    setShowReviewForm(true)
  }

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <Link to="/evaluations" className="flex items-center gap-2 text-blue-400 hover:text-blue-300 mb-6">
          <ArrowLeft className="w-4 h-4" /> 返回评估列表
        </Link>

        {}
        <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6 mb-6">
          <div className="flex items-start justify-between flex-wrap gap-4">
            <div className="flex items-center gap-4">
              <div className="w-14 h-14 bg-gradient-to-br from-purple-500 to-pink-600 rounded-xl flex items-center justify-center">
                <ClipboardCheck className="w-7 h-7 text-white" />
              </div>
              <div>
                <h1 className="text-2xl font-bold text-white">{evaluation.name}</h1>
                <div className="flex items-center gap-2 mt-2">
                  <span className={`inline-flex items-center gap-1 text-xs px-2 py-1 rounded-full ${statusStyle.bg} ${statusStyle.text}`}>
                    {StatusIcon && <StatusIcon className="w-3 h-3" />}
                    {statusStyle.label}
                  </span>
                  <span className={`inline-flex items-center text-xs px-2 py-1 rounded-full ${modeStyle.bg} ${modeStyle.text}`}>
                    {modeStyle.label}
                  </span>
                  {report?.verdict && (
                    <span className={`inline-flex items-center gap-1 text-xs px-2 py-1 rounded-full ${vStyle.bg} ${vStyle.text}`}>
                      {VerdictIcon && <VerdictIcon className="w-3 h-3" />}
                      {vStyle.label}
                    </span>
                  )}
                </div>
              </div>
            </div>

            {}
            <div className="flex flex-wrap gap-3">
              {isJudgeMode && !isFinalized && (evaluation.judge_mode === 'human' || evaluation.judge_mode === 'hybrid') && (
                <button
                  onClick={initReviewForm}
                  className="flex items-center gap-2 bg-purple-600 hover:bg-purple-700 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors"
                >
                  <User className="w-4 h-4" /> 提交评审
                </button>
              )}
              {isJudgeMode && !isFinalized && (evaluation.judge_mode === 'llm' || evaluation.judge_mode === 'hybrid') && (
                <button
                  onClick={runLLMJudge}
                  disabled={actionLoading}
                  className="flex items-center gap-2 bg-cyan-600 hover:bg-cyan-700 disabled:opacity-50 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors"
                >
                  <Bot className="w-4 h-4" /> {actionLoading ? '评审中...' : 'LLM 评审'}
                </button>
              )}
              {isJudgeMode && !isFinalized && (
                <button
                  onClick={finalize}
                  disabled={actionLoading || reviews.length === 0}
                  className="flex items-center gap-2 bg-green-600 hover:bg-green-700 disabled:opacity-50 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors"
                  title={reviews.length === 0 ? '需至少一条评审才能完成' : '聚合评审并完成评估'}
                >
                  <CheckCircle className="w-4 h-4" /> 完成评估
                </button>
              )}
              <button
                onClick={fetchData}
                className="flex items-center gap-2 bg-slate-700 hover:bg-slate-600 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors"
              >
                <RefreshCw className="w-4 h-4" /> 刷新
              </button>
            </div>
          </div>

          {}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mt-6 pt-6 border-t border-slate-700">
            {evaluation.task && (
              <div>
                <p className="text-xs text-slate-500">任务</p>
                <p className="text-sm text-slate-300">{evaluation.task}</p>
              </div>
            )}
            {evaluation.dataset && (
              <div>
                <p className="text-xs text-slate-500">数据集</p>
                <p className="text-sm text-slate-300">{evaluation.dataset}</p>
              </div>
            )}
            {evaluation.model_id && (
              <div>
                <p className="text-xs text-slate-500">模型</p>
                <p className="text-sm text-slate-300 truncate" title={evaluation.model_id}>{evaluation.model_id}</p>
              </div>
            )}
            {evaluation.experiment_id && (
              <div>
                <p className="text-xs text-slate-500">实验/运行</p>
                <p className="text-sm text-slate-300 truncate" title={`${evaluation.experiment_id}/${evaluation.run_id}`}>
                  {evaluation.experiment_id}/{evaluation.run_id}
                </p>
              </div>
            )}
          </div>

          {}
          {report?.overall !== undefined && report.overall > 0 && (
            <div className="mt-4 flex items-center gap-3">
              <span className="text-sm text-slate-400">综合得分</span>
              <span className={`text-2xl font-bold ${evaluation.passed ? 'text-green-400' : 'text-red-400'}`}>
                {(report.overall * 100).toFixed(1)}
              </span>
              <span className="text-slate-500 text-sm">/ 100</span>
            </div>
          )}
        </div>

        {}
        {dims.length > 0 && (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
            <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6">
              <h2 className="text-lg font-bold text-white mb-4">维度评分雷达图</h2>
              <ResponsiveContainer width="100%" height={320}>
                <RadarChart data={dims}>
                  <PolarGrid stroke="#334155" />
                  <PolarAngleAxis dataKey="name" tick={{ fill: '#94a3b8', fontSize: 12 }} />
                  <PolarRadiusAxis domain={[0, 1]} tick={{ fill: '#64748b', fontSize: 10 }} />
                  <Radar name="平均分" dataKey="average" stroke="#8b5cf6" fill="#8b5cf6" fillOpacity={0.4} />
                  <Tooltip
                    contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #334155', borderRadius: '8px' }}
                    formatter={(v) => [(v * 100).toFixed(1) + '分', '平均分']}
                  />
                </RadarChart>
              </ResponsiveContainer>
            </div>

            <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6">
              <h2 className="text-lg font-bold text-white mb-4">各评审类型对比</h2>
              <ResponsiveContainer width="100%" height={320}>
                <BarChart data={dims}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
                  <XAxis dataKey="name" tick={{ fill: '#94a3b8', fontSize: 11 }} />
                  <YAxis domain={[0, 1]} tick={{ fill: '#64748b', fontSize: 10 }} />
                  <Tooltip
                    contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #334155', borderRadius: '8px' }}
                    formatter={(v) => (v * 100).toFixed(1) + '分'}
                  />
                  <Legend wrapperStyle={{ fontSize: 12 }} />
                  {Object.entries(byJudgeType).map(([type, _], idx) => (
                    <Bar
                      key={type}
                      dataKey={`by_judge_type.${type}`}
                      name={type === 'human' ? '人工评审' : type === 'llm' ? 'LLM 评审' : type}
                      fill={type === 'human' ? '#a855f7' : '#06b6d4'}
                      radius={[4, 4, 0, 0]}
                    />
                  ))}
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
        )}

        {}
        {dims.length > 0 && (
          <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6 mb-6">
            <h2 className="text-lg font-bold text-white mb-4">维度明细</h2>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-slate-400 border-b border-slate-700">
                    <th className="text-left py-3 px-4">维度</th>
                    <th className="text-left py-3 px-4">说明</th>
                    <th className="text-right py-3 px-4">权重</th>
                    <th className="text-right py-3 px-4">平均分</th>
                    {Object.keys(byJudgeType).map((type) => (
                      <th key={type} className="text-right py-3 px-4">
                        {type === 'human' ? '人工' : type === 'llm' ? 'LLM' : type}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {dims.map((d) => (
                    <tr key={d.name} className="border-b border-slate-700/50 hover:bg-slate-700/30">
                      <td className="py-3 px-4 text-white font-medium">{d.name}</td>
                      <td className="py-3 px-4 text-slate-400">{d.description || '—'}</td>
                      <td className="py-3 px-4 text-right text-slate-300">{(d.weight * 100).toFixed(0)}%</td>
                      <td className="py-3 px-4 text-right">
                        <span className={`font-bold ${(d.average || 0) >= 0.6 ? 'text-green-400' : 'text-orange-400'}`}>
                          {((d.average || 0) * 100).toFixed(1)}
                        </span>
                      </td>
                      {Object.keys(byJudgeType).map((type) => (
                        <td key={type} className="py-3 px-4 text-right text-slate-300">
                          {d.by_judge_type?.[type] !== undefined
                            ? (d.by_judge_type[type] * 100).toFixed(1)
                            : '—'}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {}
        <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6">
          <h2 className="text-lg font-bold text-white mb-4">
            评审记录 <span className="text-slate-500 text-sm font-normal">({reviews.length})</span>
          </h2>
          {reviews.length === 0 ? (
            <div className="text-center text-slate-500 py-8">
              {isJudgeMode ? '暂无评审记录，点击上方按钮提交评审或触发 LLM 评审' : '阈值裁决模式不产生评审记录'}
            </div>
          ) : (
            <div className="space-y-3">
              {reviews.map((rv) => {
                const scores = typeof rv.scores === 'string' ? safeParse(rv.scores) : rv.scores
                const isHuman = rv.judge_type === 'human'
                return (
                  <div key={rv.id} className="bg-slate-700/40 rounded-xl p-4 border border-slate-700">
                    <div className="flex items-center justify-between mb-3">
                      <div className="flex items-center gap-2">
                        {isHuman ? (
                          <User className="w-4 h-4 text-purple-400" />
                        ) : (
                          <Bot className="w-4 h-4 text-cyan-400" />
                        )}
                        <span className="text-sm font-medium text-white">
                          {isHuman ? '人工评审' : 'LLM 评审'}
                        </span>
                        <span className="text-xs text-slate-500">{rv.judge_id}</span>
                      </div>
                      <span className="text-xs text-slate-500">
                        {new Date(rv.created_at).toLocaleString()}
                      </span>
                    </div>
                    {scores && Object.keys(scores).length > 0 && (
                      <div className="flex flex-wrap gap-2 mb-2">
                        {Object.entries(scores).map(([dim, score]) => (
                          <span
                            key={dim}
                            className={`text-xs px-2 py-1 rounded-md ${
                              score >= 0.6 ? 'bg-green-500/20 text-green-400' : 'bg-orange-500/20 text-orange-400'
                            }`}
                          >
                            {dim}: {(score * 100).toFixed(0)}分
                          </span>
                        ))}
                      </div>
                    )}
                    {rv.comment && (
                      <p className="text-sm text-slate-400 mt-2">{rv.comment}</p>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>

        {}
        {showReviewForm && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-lg border border-slate-700 shadow-2xl">
              <h2 className="text-xl font-bold text-white mb-4">提交人工评审</h2>
              <form onSubmit={submitReview} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">
                    各维度评分 JSON
                  </label>
                  <textarea
                    value={reviewForm.scores}
                    onChange={(e) => setReviewForm({ ...reviewForm, scores: e.target.value })}
                    rows={6}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-purple-500 font-mono text-sm"
                    placeholder='{"accuracy": 0.9, "fluency": 0.8}'
                    required
                  />
                  <p className="text-xs text-slate-500 mt-1">值域 [0, 1]，各维度独立打分</p>
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">评审意见（可选）</label>
                  <textarea
                    value={reviewForm.comment}
                    onChange={(e) => setReviewForm({ ...reviewForm, comment: e.target.value })}
                    rows={3}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-purple-500"
                  />
                </div>
                <div className="flex gap-3 mt-6">
                  <button
                    type="button"
                    onClick={() => setShowReviewForm(false)}
                    className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors"
                  >
                    取消
                  </button>
                  <button
                    type="submit"
                    disabled={actionLoading}
                    className="flex-1 bg-gradient-to-r from-purple-600 to-pink-600 hover:from-purple-700 hover:to-pink-700 disabled:opacity-50 text-white px-6 py-3 rounded-xl font-medium transition-all"
                  >
                    {actionLoading ? '提交中...' : '提交评审'}
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

const safeParse = (str) => {
  try {
    return JSON.parse(str)
  } catch {
    return null
  }
}

export default EvaluationReport
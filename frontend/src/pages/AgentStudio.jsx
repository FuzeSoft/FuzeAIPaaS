import React, { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../auth'
import {
  Workflow, Plus, Play, GitBranch, UserCheck, Cpu, Wrench, BookOpen, Trash2, CheckCircle2, XCircle, Loader2,
} from 'lucide-react'

const NODE_META = {
  llm_call: { icon: Cpu, label: 'LLM 调用', color: 'from-blue-500 to-cyan-500' },
  tool_call: { icon: Wrench, label: '工具调用', color: 'from-amber-500 to-orange-500' },
  rag_retrieve: { icon: BookOpen, label: '知识检索', color: 'from-emerald-500 to-green-500' },
  condition: { icon: GitBranch, label: '条件分支', color: 'from-purple-500 to-violet-500' },
  human_review: { icon: UserCheck, label: '人工审核', color: 'from-rose-500 to-pink-500' },
  sub_agent: { icon: Workflow, label: '子代理', color: 'from-teal-500 to-cyan-500' },
}

const statusStyle = (s) => {
  switch (s) {
    case 'succeeded': return 'text-emerald-400 bg-emerald-500/10'
    case 'failed': return 'text-rose-400 bg-rose-500/10'
    case 'paused': return 'text-amber-400 bg-amber-500/10'
    case 'running': return 'text-blue-400 bg-blue-500/10'
    default: return 'text-slate-400 bg-slate-500/10'
  }
}

const AgentStudio = () => {
  const [agents, setAgents] = useState([])
  const [selected, setSelected] = useState(null) 
  const [loading, setLoading] = useState(true)
  const [enabled, setEnabled] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ name: '', description: '' })
  const [creating, setCreating] = useState(false)

  const [runInput, setRunInput] = useState('')
  const [run, setRun] = useState(null)
  const [running, setRunning] = useState(false)
  const [resumeDecision, setResumeDecision] = useState('')
  const [resuming, setResuming] = useState(false)

  const fetchAgents = useCallback(async () => {
    try {
      const res = await apiFetch('/api/v1/agents')
      if (res.status === 501) {
        setEnabled(false)
        setAgents([])
        return
      }
      const data = await res.json()
      setAgents(Array.isArray(data.agents) ? data.agents : [])
    } catch (e) {
      console.error('fetch agents failed', e)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchAgents() }, [fetchAgents])

  const selectAgent = async (a) => {
    setRun(null)
    try {
      const res = await apiFetch(`/api/v1/agents/${a.id}`)
      if (res.ok) {
        const data = await res.json()
        setSelected(data.agent)
        return
      }
    } catch (e) {  }
    setSelected(a)
  }

  const handleCreate = async (e) => {
    e.preventDefault()
    if (!form.name) return
    setCreating(true)
    
    const dag = {
      nodes: [
        { id: 'n1', type: 'llm_call', ref: 'gpt-4', config: { prompt: '请处理用户请求：{{input}}' } },
        { id: 'n2', type: 'llm_call', ref: 'gpt-4', config: { prompt: '基于上一步结果总结：{{n1}}' } },
      ],
      edges: { n1: ['n2'] },
    }
    try {
      const res = await apiFetch('/api/v1/agents', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: form.name, description: form.description, dag }),
      })
      if (res.ok) {
        setShowCreate(false)
        setForm({ name: '', description: '' })
        fetchAgents()
      } else {
        alert('创建失败')
      }
    } finally {
      setCreating(false)
    }
  }

  const handleCompile = async (id) => {
    try {
      const res = await apiFetch(`/api/v1/agents/${id}/compile`, { method: 'POST' })
      if (res.ok) {
        selectAgent({ id })
        fetchAgents()
      } else {
        const d = await res.json().catch(() => ({}))
        alert('编译失败：' + (d.error || res.status))
      }
    } catch (e) { console.error(e) }
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除该 Agent？关联运行记录将一并清除。')) return
    try {
      await apiFetch(`/api/v1/agents/${id}`, { method: 'DELETE' })
      if (selected && selected.id === id) setSelected(null)
      fetchAgents()
    } catch (e) { console.error(e) }
  }

  const handleRun = async () => {
    if (!selected) return
    setRunning(true)
    setRun(null)
    try {
      const res = await apiFetch(`/api/v1/agents/${selected.id}/runs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ input: runInput }),
      })
      if (res.ok) {
        const d = await res.json()
        setRun(d.run)
      } else {
        const d = await res.json().catch(() => ({}))
        alert('运行失败：' + (d.error || res.status))
      }
    } catch (e) {
      
      console.error(e)
      alert('运行失败：' + (e.message || '网络错误'))
    } finally {
      setRunning(false)
    }
  }

  const handleResume = async () => {
    if (!run) return
    setResuming(true)
    try {
      const res = await apiFetch(`/api/v1/agents/${selected.id}/runs/${run.id}/resume`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ decision: resumeDecision }),
      })
      if (res.ok) {
        const d = await res.json()
        setRun(d.run)
        setResumeDecision('')
      } else {
        const d = await res.json().catch(() => ({}))
        alert('恢复失败：' + (d.error || res.status))
      }
    } catch (e) {
      console.error(e)
      alert('恢复失败：' + (e.message || '网络错误'))
    } finally {
      setResuming(false)
    }
  }

  if (!enabled) {
    return (
      <div className="p-8">
        <div className="flex items-center gap-3 mb-6">
          <Workflow className="w-7 h-7 text-blue-400" />
          <h1 className="text-2xl font-bold text-white">Agent 编排</h1>
        </div>
        <div className="bg-slate-900/60 border border-slate-800 rounded-xl p-8 text-center text-slate-400">
          <p>Agent 编排模块未启用（后端未接入 AgentRepository）。</p>
          <p className="text-sm mt-2">请参考 P6 路线图 B3 完成适配器装配后重试。</p>
        </div>
      </div>
    )
  }

  return (
    <div className="p-8">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Workflow className="w-7 h-7 text-blue-400" />
          <h1 className="text-2xl font-bold text-white">Agent 编排</h1>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded-lg transition-colors"
        >
          <Plus className="w-4 h-4" /> 新建 Agent
        </button>
      </div>

      <div className="grid grid-cols-12 gap-6">
        {}
        <div className="col-span-4">
          <div className="bg-slate-900/60 border border-slate-800 rounded-xl p-4">
            <h2 className="text-sm font-semibold text-slate-300 mb-3">Agents</h2>
            {loading ? (
              <p className="text-slate-500 text-sm">加载中…</p>
            ) : agents.length === 0 ? (
              <p className="text-slate-500 text-sm">暂无 Agent，点击右上角新建。</p>
            ) : (
              <ul className="space-y-2">
                {agents.map((a) => (
                  <li
                    key={a.id}
                    onClick={() => selectAgent(a)}
                    className={`cursor-pointer rounded-lg p-3 border transition-colors ${
                      selected && selected.id === a.id
                        ? 'border-blue-500 bg-blue-500/10'
                        : 'border-slate-800 hover:border-slate-700 bg-slate-900/40'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-white font-medium truncate">{a.name}</span>
                      <span className={`text-[11px] px-2 py-0.5 rounded-full ${statusStyle(a.status)}`}>{a.status}</span>
                    </div>
                    <p className="text-xs text-slate-400 mt-1 truncate">{a.description || '（无描述）'}</p>
                    <p className="text-[11px] text-slate-500 mt-1">{a.dag?.nodes?.length || 0} 节点</p>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>

        {}
        <div className="col-span-8 space-y-6">
          {!selected ? (
            <div className="bg-slate-900/60 border border-slate-800 rounded-xl p-8 text-center text-slate-500">
              从左侧选择一个 Agent 查看其工作流画布。
            </div>
          ) : (
            <>
              <div className="bg-slate-900/60 border border-slate-800 rounded-xl p-5">
                <div className="flex items-center justify-between mb-4">
                  <div>
                    <h2 className="text-lg font-semibold text-white">{selected.name}</h2>
                    <p className="text-xs text-slate-400">{selected.id} · {selected.status}</p>
                  </div>
                  <div className="flex gap-2">
                    {selected.status !== 'compiled' && selected.status !== 'published' && (
                      <button
                        onClick={() => handleCompile(selected.id)}
                        className="flex items-center gap-1 bg-purple-600 hover:bg-purple-500 text-white px-3 py-1.5 rounded-lg text-sm"
                      >
                        <CheckCircle2 className="w-4 h-4" /> 编译
                      </button>
                    )}
                    <button
                      onClick={() => handleDelete(selected.id)}
                      className="flex items-center gap-1 bg-rose-600/80 hover:bg-rose-500 text-white px-3 py-1.5 rounded-lg text-sm"
                    >
                      <Trash2 className="w-4 h-4" /> 删除
                    </button>
                  </div>
                </div>

                {}
                <div className="flex flex-wrap gap-3">
                  {(selected.dag?.nodes || []).map((n) => {
                    const meta = NODE_META[n.type] || { icon: Cpu, label: n.type, color: 'from-slate-500 to-slate-600' }
                    const Icon = meta.icon
                    const deps = Object.entries(selected.dag?.edges || {})
                      .filter(([, tos]) => tos.includes(n.id)).map(([from]) => from)
                    return (
                      <div key={n.id} className={`relative rounded-xl p-3 bg-gradient-to-br ${meta.color} text-white w-56 shadow-lg`}>
                        <div className="flex items-center gap-2 mb-1">
                          <Icon className="w-4 h-4" />
                          <span className="text-sm font-semibold">{meta.label}</span>
                        </div>
                        <p className="text-[11px] opacity-90 font-mono">{n.id}{n.ref ? ` · ${n.ref}` : ''}</p>
                        {deps.length > 0 && (
                          <p className="text-[10px] opacity-75 mt-1">← {deps.join(', ')}</p>
                        )}
                      </div>
                    )
                  })}
                </div>
              </div>

              {}
              <div className="bg-slate-900/60 border border-slate-800 rounded-xl p-5">
                <h3 className="text-sm font-semibold text-slate-300 mb-3">运行</h3>
                <div className="flex gap-2 mb-4">
                  <input
                    value={runInput}
                    onChange={(e) => setRunInput(e.target.value)}
                    placeholder="输入初始请求…"
                    className="flex-1 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
                  />
                  <button
                    onClick={handleRun}
                    disabled={running}
                    className="flex items-center gap-1 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white px-4 py-2 rounded-lg text-sm"
                  >
                    {running ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />} 运行
                  </button>
                </div>

                {run && (
                  <div className="space-y-3">
                    <div className="flex items-center gap-2 text-sm">
                      <span className={`px-2 py-0.5 rounded-full text-[11px] ${statusStyle(run.status)}`}>{run.status}</span>
                      <span className="text-slate-400">运行 ID: {run.id}</span>
                    </div>

                    {}
                    <div className="space-y-2">
                      {(run.results || []).map((r, i) => (
                        <div key={i} className="rounded-lg border border-slate-800 bg-slate-900/40 p-3">
                          <div className="flex items-center justify-between mb-1">
                            <span className="text-xs font-mono text-slate-300">{r.node_id}</span>
                            {r.status === 'ok'
                              ? <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                              : <XCircle className="w-4 h-4 text-rose-400" />}
                          </div>
                          <pre className="text-xs text-slate-300 whitespace-pre-wrap break-words max-h-40 overflow-auto">{r.output || r.error || '（无输出）'}</pre>
                        </div>
                      ))}
                    </div>

                    {}
                    {run.status === 'paused' && (
                      <div className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3">
                        <p className="text-sm text-amber-300 mb-2">⏸ 等待人工审核：{run.pause_prompt}</p>
                        <div className="flex gap-2">
                          <input
                            value={resumeDecision}
                            onChange={(e) => setResumeDecision(e.target.value)}
                            placeholder="输入审核决定…"
                            className="flex-1 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-500"
                          />
                          <button
                            onClick={handleResume}
                            disabled={resuming}
                            className="flex items-center gap-1 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white px-4 py-2 rounded-lg text-sm"
                          >
                            {resuming ? <Loader2 className="w-4 h-4 animate-spin" /> : <UserCheck className="w-4 h-4" />} 恢复
                          </button>
                        </div>
                      </div>
                    )}

                    {run.status === 'succeeded' && (
                      <div className="rounded-lg border border-emerald-500/40 bg-emerald-500/5 p-3">
                        <p className="text-xs text-emerald-300 mb-1">最终输出</p>
                        <pre className="text-sm text-slate-200 whitespace-pre-wrap break-words">{run.final_output}</pre>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>

      {}
      {showCreate && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
          <form onSubmit={handleCreate} className="bg-slate-900 border border-slate-700 rounded-xl p-6 w-96">
            <h3 className="text-lg font-semibold text-white mb-4">新建 Agent</h3>
            <label className="block text-sm text-slate-400 mb-1">名称</label>
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white mb-3 focus:outline-none focus:border-blue-500"
              required
            />
            <label className="block text-sm text-slate-400 mb-1">描述</label>
            <textarea
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white mb-4 focus:outline-none focus:border-blue-500"
              rows={3}
            />
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setShowCreate(false)} className="text-sm text-slate-400 hover:text-white px-4 py-2">取消</button>
              <button type="submit" disabled={creating} className="text-sm bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white px-4 py-2 rounded-lg">
                {creating ? '创建中…' : '创建'}
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  )
}

export default AgentStudio
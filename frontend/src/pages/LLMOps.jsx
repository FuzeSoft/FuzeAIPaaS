import React, { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../auth'
import {
  Network, Coins, Activity, MessageSquare, BookOpen, ScrollText,
  Plus, Trash2, RefreshCw, Cpu,
} from 'lucide-react'

const TABS = [
  { key: 'routes', label: '网关路由', icon: Network },
  { key: 'quota', label: 'Token 配额', icon: Coins },
  { key: 'usage', label: '用量统计', icon: Activity },
  { key: 'traces', label: '调用链路', icon: ScrollText },
  { key: 'prompts', label: '提示词', icon: MessageSquare },
  { key: 'knowledge', label: '知识库', icon: BookOpen },
]

const LLMOps = () => {
  const [tab, setTab] = useState('routes')

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="mb-8">
          <h1 className="text-3xl font-bold text-white mb-2">大模型专属能力（LLMOps）</h1>
          <p className="text-slate-400">
            推理网关 · Token 计量与成本 · 提示词工程 · 护栏 · RAG 知识库 · 微调与评估闭环 · 调用链路 Trace
          </p>
        </div>
        <div className="flex flex-wrap gap-2 mb-8">
          {TABS.map((t) => {
            const Icon = t.icon
            const active = tab === t.key
            return (
              <button
                key={t.key}
                onClick={() => setTab(t.key)}
                className={`flex items-center gap-2 px-4 py-2.5 rounded-xl font-medium transition-all ${
                  active
                    ? 'bg-gradient-to-r from-blue-600 to-purple-600 text-white shadow-lg shadow-blue-500/25'
                    : 'bg-slate-800 text-slate-300 hover:bg-slate-700 hover:text-white'
                }`}
              >
                <Icon className="w-4 h-4" />
                {t.label}
              </button>
            )
          })}
        </div>
        {tab === 'routes' && <RoutesTab />}
        {tab === 'quota' && <QuotaTab />}
        {tab === 'usage' && <UsageTab />}
        {tab === 'traces' && <TracesTab />}
        {tab === 'prompts' && <PromptsTab />}
        {tab === 'knowledge' && <KnowledgeTab />}
      </div>
    </div>
  )
}

async function getJSON(path) {
  const res = await apiFetch(path)
  if (!res.ok) throw new Error(`请求失败 ${res.status}`)
  return res.json()
}
async function sendJSON(path, method, body) {
  const res = await apiFetch(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const text = await res.text()
  if (!res.ok) throw new Error(text || `请求失败 ${res.status}`)
  return text ? JSON.parse(text) : {}
}

const nsToMs = (ns) => ((ns ?? 0) / 1e6).toFixed(1)
const tps = (v) => ((v ?? 0)).toFixed(1)

const RoutesTab = () => {
  const [routes, setRoutes] = useState([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [form, setForm] = useState({ model: '', strategy: 'priority', backends: [{ name: '', endpoint: '' }] })

  const load = useCallback(async () => {
    try {
      const d = await getJSON('/api/v1/llm/routes')
      setRoutes(d.routes || [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const setBackend = (i, field, val) =>
    setForm((f) => {
      const backends = f.backends.map((b, idx) => (idx === i ? { ...b, [field]: val } : b))
      return { ...f, backends }
    })
  const addBackend = () => setForm((f) => ({ ...f, backends: [...f.backends, { name: '', endpoint: '' }] }))
  const removeBackend = (i) =>
    setForm((f) => ({ ...f, backends: f.backends.filter((_, idx) => idx !== i) }))

  const handleSave = async (e) => {
    e.preventDefault()
    try {
      const backends = form.backends
        .filter((b) => b.endpoint)
        .map((b) => ({ name: b.name || b.endpoint, endpoint: b.endpoint, weight: 1, healthy: true }))
      await sendJSON('/api/v1/llm/routes', 'PUT', { model: form.model, strategy: form.strategy, backends })
      setShowModal(false)
      setForm({ model: '', strategy: 'priority', backends: [{ name: '', endpoint: '' }] })
      load()
    } catch (e) {
      alert('保存失败：' + e.message)
    }
  }

  const handleDelete = async (model) => {
    if (!confirm(`删除路由 ${model}？`)) return
    try {
      await apiFetch(`/api/v1/llm/routes/${encodeURIComponent(model)}`, { method: 'DELETE' })
      load()
    } catch (e) {
      console.error(e)
    }
  }

  return (
    <Panel loading={loading} onRefresh={load} onAdd={() => setShowModal(true)} addLabel="新建路由">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
        {routes.map((rt) => (
          <Card key={rt.model} title={rt.model} subtitle={`策略: ${rt.strategy}`} onDelete={() => handleDelete(rt.model)}>
            <ul className="space-y-2">
              {(rt.backends || []).map((b, i) => (
                <li key={i} className="flex items-center justify-between text-sm">
                  <span className="text-slate-300">{b.name}</span>
                  <span className="text-slate-400 truncate max-w-[200px]" title={b.endpoint}>{b.endpoint}</span>
                </li>
              ))}
              {(rt.backends || []).length === 0 && <li className="text-slate-500 text-sm">无后端</li>}
            </ul>
          </Card>
        ))}
      </div>

      {showModal && (
        <Modal title="新建网关路由" onClose={() => setShowModal(false)}>
          <form onSubmit={handleSave} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-2">对外模型名</label>
              <input className="input" value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} required placeholder="gpt-4o" />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-2">路由策略</label>
              <select className="input" value={form.strategy} onChange={(e) => setForm({ ...form, strategy: e.target.value })}>
                <option value="priority">优先级 (priority)</option>
                <option value="round_robin">轮询 (round_robin)</option>
                <option value="weighted">加权 (weighted)</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-2">后端列表</label>
              {form.backends.map((b, i) => (
                <div key={i} className="flex gap-2 mb-2">
                  <input className="input flex-1" value={b.name} onChange={(e) => setBackend(i, 'name', e.target.value)} placeholder="名称" />
                  <input className="input flex-1" value={b.endpoint} onChange={(e) => setBackend(i, 'endpoint', e.target.value)} placeholder="https://..." />
                  <button type="button" onClick={() => removeBackend(i)} className="px-3 text-red-400 hover:bg-slate-700 rounded-lg">×</button>
                </div>
              ))}
              <button type="button" onClick={addBackend} className="text-blue-400 text-sm hover:text-blue-300">+ 添加后端</button>
            </div>
            <div className="flex gap-3 mt-4">
              <button type="button" onClick={() => setShowModal(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-4 py-2.5 rounded-xl">取消</button>
              <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 text-white px-4 py-2.5 rounded-xl">保存</button>
            </div>
          </form>
        </Modal>
      )}
    </Panel>
  )
}

const QuotaTab = () => {
  const [quota, setQuota] = useState(null)
  const [loading, setLoading] = useState(true)
  const [form, setForm] = useState({ limit_tokens: 0, limit_cost: 0 })
  const [saving, setSaving] = useState(false)

  const load = useCallback(async () => {
    try {
      const d = await getJSON('/api/v1/llm/quota')
      setQuota(d)
      setForm({ limit_tokens: d.limit_tokens || 0, limit_cost: d.limit_cost || 0 })
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => { load() }, [load])

  const handleSave = async (e) => {
    e.preventDefault()
    setSaving(true)
    try {
      await sendJSON('/api/v1/llm/quota', 'PUT', {
        limit_tokens: Number(form.limit_tokens),
        limit_cost: Number(form.limit_cost),
      })
      load()
    } catch (e) {
      alert('保存失败：' + e.message)
    } finally {
      setSaving(false)
    }
  }

  const pct = (used, limit) => (limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : 0)

  return (
    <Panel loading={loading} onRefresh={load}>
      {quota && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div className="bg-slate-800 rounded-2xl border border-slate-700 p-6">
            <h3 className="text-white font-semibold mb-4 flex items-center gap-2"><Coins className="w-5 h-5 text-yellow-400" /> 配额概览</h3>
            <Bar label="Token 用量" used={quota.used_tokens || 0} limit={quota.limit_tokens || 0} unit="tok" />
            <Bar label="成本用量" used={quota.used_cost || 0} limit={quota.limit_cost || 0} unit="" money />
          </div>
          <div className="bg-slate-800 rounded-2xl border border-slate-700 p-6">
            <h3 className="text-white font-semibold mb-4">调整上限</h3>
            <form onSubmit={handleSave} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">Token 上限</label>
                <input type="number" className="input" value={form.limit_tokens} onChange={(e) => setForm({ ...form, limit_tokens: e.target.value })} />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">成本上限（元）</label>
                <input type="number" step="0.01" className="input" value={form.limit_cost} onChange={(e) => setForm({ ...form, limit_cost: e.target.value })} />
              </div>
              <button type="submit" disabled={saving} className="w-full bg-gradient-to-r from-blue-600 to-purple-600 text-white px-4 py-2.5 rounded-xl disabled:opacity-50">
                {saving ? '保存中...' : '保存配额'}
              </button>
            </form>
          </div>
        </div>
      )}
    </Panel>
  )
}

const Bar = ({ label, used, limit, unit, money }) => {
  const p = limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : 0
  return (
    <div className="mb-4">
      <div className="flex justify-between text-sm mb-1">
        <span className="text-slate-400">{label}</span>
        <span className="text-slate-300">{money ? `¥${used}` : used}{unit} / {money ? `¥${limit}` : limit}{unit}</span>
      </div>
      <div className="h-2.5 bg-slate-700 rounded-full overflow-hidden">
        <div className={`h-full rounded-full ${p > 90 ? 'bg-red-500' : p > 70 ? 'bg-yellow-500' : 'bg-gradient-to-r from-blue-500 to-purple-500'}`} style={{ width: `${p}%` }} />
      </div>
    </div>
  )
}

const UsageTab = () => {
  const [sum, setSum] = useState(null)
  const [records, setRecords] = useState([])
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    try {
      const [s, r] = await Promise.all([
        getJSON('/api/v1/llm/usage/sum'),
        getJSON('/api/v1/llm/usage?limit=50'),
      ])
      setSum(s)
      setRecords(r.records || [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => { load() }, [load])

  return (
    <Panel loading={loading} onRefresh={load}>
      {sum && (
        <>
          <div className="grid grid-cols-2 md:grid-cols-3 gap-4 mb-6">
            <Stat label="总 Token" value={sum.usage?.total_tokens ?? 0} />
            <Stat label="总成本" value={`¥${sum.cost?.toFixed?.(4) ?? sum.cost ?? 0}`} />
            <Stat label="调用次数" value={records.length} />
          </div>
          <div className="bg-slate-800 rounded-2xl border border-slate-700 overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-slate-900/60 text-slate-400">
                <tr>
                  <th className="text-left px-4 py-3">模型</th>
                  <th className="text-left px-4 py-3">后端</th>
                  <th className="text-right px-4 py-3">Token</th>
                  <th className="text-right px-4 py-3">成本</th>
                  <th className="text-right px-4 py-3">耗时(ms)</th>
                  <th className="text-right px-4 py-3">状态</th>
                </tr>
              </thead>
              <tbody>
                {records.map((r) => (
                  <tr key={r.id} className="border-t border-slate-700">
                    <td className="px-4 py-3 text-slate-200 truncate max-w-[200px]" title={r.model}>{r.model}</td>
                    <td className="px-4 py-3 text-slate-400 truncate max-w-[200px]" title={r.backend}>{r.backend}</td>
                    <td className="px-4 py-3 text-right text-slate-300">{r.total_tokens}</td>
                    <td className="px-4 py-3 text-right text-slate-300">¥{r.cost}</td>
                    <td className="px-4 py-3 text-right text-slate-300">{r.latency_ms}</td>
                    <td className="px-4 py-3 text-right">{r.success ? <span className="text-green-400">成功</span> : <span className="text-red-400">失败</span>}</td>
                  </tr>
                ))}
                {records.length === 0 && (
                  <tr><td colSpan="6" className="text-center text-slate-500 py-8">暂无用量数据</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </Panel>
  )
}

const TracesTab = () => {
  const [traces, setTraces] = useState([])
  const [loading, setLoading] = useState(true)
  const [open, setOpen] = useState(null)

  const load = useCallback(async () => {
    try {
      const d = await getJSON('/api/v1/llm/traces?limit=50')
      setTraces(d.traces || [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => { load() }, [load])

  return (
    <Panel loading={loading} onRefresh={load}>
      <div className="space-y-3">
        {traces.map((t) => (
          <div key={t.id} className="bg-slate-800 rounded-2xl border border-slate-700 p-5">
            <button className="w-full text-left" onClick={() => setOpen(open === t.id ? null : t.id)}>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Cpu className="w-5 h-5 text-cyan-400" />
                  <span className="text-white font-medium">{t.model}</span>
                  {t.error ? <span className="text-xs text-red-400 bg-red-500/10 px-2 py-0.5 rounded">error</span> : <span className="text-xs text-green-400 bg-green-500/10 px-2 py-0.5 rounded">ok</span>}
                </div>
                <span className="text-xs text-slate-400">TTFT {nsToMs(t.latency?.ttft)}ms · TPS {tps(t.latency?.tokens_per_second)}</span>
              </div>
            </button>
            {open === t.id && (
              <div className="mt-4 pt-4 border-t border-slate-700 grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
                <Metric label="Prompt Tokens" value={t.usage?.prompt_tokens} />
                <Metric label="Completion Tokens" value={t.usage?.completion_tokens} />
                <Metric label="Total Tokens" value={t.usage?.total_tokens} />
                <Metric label="成本" value={`¥${t.cost}`} />
                <Metric label="TTFT" value={`${nsToMs(t.latency?.ttft)}ms`} />
                <Metric label="TPOT" value={`${nsToMs(t.latency?.tpot)}ms`} />
                <Metric label="Total" value={`${nsToMs(t.latency?.total)}ms`} />
                <Metric label="TPS" value={tps(t.latency?.tokens_per_second)} />
                <div className="col-span-2 md:col-span-4 mt-2">
                  <div className="text-slate-400 text-xs mb-1">Spans ({t.spans?.length || 0})</div>
                  <ul className="space-y-1">
                    {(t.spans || []).map((s, i) => (
                      <li key={i} className="text-slate-300 text-xs flex justify-between">
                        <span>{s.name}</span>
                        <span className="text-slate-500">{(s.elapsed / 1e6).toFixed(1)}ms</span>
                      </li>
                    ))}
                    {(t.spans || []).length === 0 && <li className="text-slate-500 text-xs">无 span</li>}
                  </ul>
                </div>
              </div>
            )}
          </div>
        ))}
        {traces.length === 0 && <div className="text-center text-slate-500 py-12">暂无调用链路</div>}
      </div>
    </Panel>
  )
}

const PromptsTab = () => {
  const [prompts, setPrompts] = useState([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ name: '', content: '' })
  const [detail, setDetail] = useState(null)

  const load = useCallback(async () => {
    try {
      const d = await getJSON('/api/v1/llm/prompts')
      setPrompts(d.prompts || [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => { load() }, [load])

  const handleCreate = async (e) => {
    e.preventDefault()
    try {
      await sendJSON('/api/v1/llm/prompts', 'POST', form)
      setShowCreate(false)
      setForm({ name: '', content: '' })
      load()
    } catch (e) {
      alert('创建失败：' + e.message)
    }
  }

  const openDetail = async (name) => {
    try {
      const p = await getJSON(`/api/v1/llm/prompts/${encodeURIComponent(name)}`)
      setDetail(p)
    } catch (e) {
      console.error(e)
    }
  }

  const addVersion = async (name, content) => {
    try {
      await sendJSON(`/api/v1/llm/prompts/${encodeURIComponent(name)}/versions`, 'POST', { content, activate: true })
      openDetail(name)
      load()
    } catch (e) {
      alert('追加版本失败：' + e.message)
    }
  }

  return (
    <Panel loading={loading} onRefresh={load} onAdd={() => setShowCreate(true)} addLabel="新建提示词">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {prompts.map((p) => (
          <Card key={p.name} title={p.name} subtitle={`v${p.active_version} · ${p.versions?.length || 0} 个版本`} onClick={() => openDetail(p.name)}>
            <p className="text-slate-400 text-sm line-clamp-3">{p.versions?.find((v) => v.version === p.active_version)?.content || '—'}</p>
          </Card>
        ))}
      </div>

      {showCreate && (
        <Modal title="新建提示词模板" onClose={() => setShowCreate(false)}>
          <form onSubmit={handleCreate} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-2">模板名</label>
              <input className="input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required placeholder="summarize" />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-2">首版内容（支持 {'{{var}}'} 变量）</label>
              <textarea className="input h-32" value={form.content} onChange={(e) => setForm({ ...form, content: e.target.value })} required />
            </div>
            <div className="flex gap-3">
              <button type="button" onClick={() => setShowCreate(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-4 py-2.5 rounded-xl">取消</button>
              <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 text-white px-4 py-2.5 rounded-xl">创建</button>
            </div>
          </form>
        </Modal>
      )}

      {detail && (
        <Modal title={`提示词：${detail.name}`} onClose={() => setDetail(null)} large>
          <div className="space-y-3">
            <p className="text-slate-400 text-xs">当前激活版本 v{detail.active_version}</p>
            {detail.versions?.map((v) => (
              <div key={v.version} className="bg-slate-900/60 rounded-xl p-4 border border-slate-700">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-sm text-slate-300 font-medium">v{v.version}{v.version === detail.active_version ? ' (激活)' : ''}</span>
                </div>
                <pre className="text-xs text-slate-300 whitespace-pre-wrap">{v.content}</pre>
              </div>
            ))}
            <form onSubmit={(e) => { e.preventDefault(); const c = e.target.content.value; if (c) addVersion(detail.name, c) }}>
              <label className="block text-sm font-medium text-slate-300 mb-2">追加新版本（追加后自动激活）</label>
              <textarea name="content" className="input h-28" placeholder="新版本内容" />
              <button className="mt-3 w-full bg-gradient-to-r from-blue-600 to-purple-600 text-white px-4 py-2.5 rounded-xl">追加版本</button>
            </form>
          </div>
        </Modal>
      )}
    </Panel>
  )
}

const KnowledgeTab = () => {
  const [bases, setBases] = useState([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ name: '', embedding_model: 'text-embedding-3-small', chunk_size: 512, overlap: 64 })
  const [detail, setDetail] = useState(null)

  const load = useCallback(async () => {
    try {
      const d = await getJSON('/api/v1/llm/knowledge')
      setBases(d.bases || [])
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => { load() }, [load])

  const handleCreate = async (e) => {
    e.preventDefault()
    try {
      await sendJSON('/api/v1/llm/knowledge', 'POST', {
        name: form.name,
        embedding_model: form.embedding_model,
        chunk_size: Number(form.chunk_size),
        overlap: Number(form.overlap),
      })
      setShowCreate(false)
      setForm({ name: '', embedding_model: 'text-embedding-3-small', chunk_size: 512, overlap: 64 })
      load()
    } catch (e) {
      alert('创建失败：' + e.message)
    }
  }

  const openDetail = async (id) => {
    try {
      const [kb, docs] = await Promise.all([
        getJSON(`/api/v1/llm/knowledge/${id}`),
        getJSON(`/api/v1/llm/knowledge/${id}/documents`),
      ])
      setDetail({ ...kb, documents: docs.documents || [] })
    } catch (e) {
      console.error(e)
    }
  }

  const addDoc = async (id, title, content) => {
    try {
      await sendJSON(`/api/v1/llm/knowledge/${id}/documents`, 'POST', { title, content, source: 'manual' })
      openDetail(id)
    } catch (e) {
      alert('添加文档失败：' + e.message)
    }
  }

  return (
    <Panel loading={loading} onRefresh={load} onAdd={() => setShowCreate(true)} addLabel="新建知识库">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {bases.map((kb) => (
          <Card key={kb.id} title={kb.name} subtitle={`${kb.embedding_model} · chunk=${kb.chunk_size}`} onClick={() => openDetail(kb.id)}>
            <p className="text-slate-400 text-sm">点击查看文档与切片</p>
          </Card>
        ))}
      </div>

      {showCreate && (
        <Modal title="新建知识库" onClose={() => setShowCreate(false)}>
          <form onSubmit={handleCreate} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-2">知识库名</label>
              <input className="input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-2">Embedding 模型</label>
              <input className="input" value={form.embedding_model} onChange={(e) => setForm({ ...form, embedding_model: e.target.value })} />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">切片大小</label>
                <input type="number" className="input" value={form.chunk_size} onChange={(e) => setForm({ ...form, chunk_size: e.target.value })} />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">重叠</label>
                <input type="number" className="input" value={form.overlap} onChange={(e) => setForm({ ...form, overlap: e.target.value })} />
              </div>
            </div>
            <div className="flex gap-3">
              <button type="button" onClick={() => setShowCreate(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-4 py-2.5 rounded-xl">取消</button>
              <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 text-white px-4 py-2.5 rounded-xl">创建</button>
            </div>
          </form>
        </Modal>
      )}

      {detail && (
        <Modal title={`知识库：${detail.name}`} onClose={() => setDetail(null)} large>
          <div className="space-y-3">
            <p className="text-slate-400 text-xs">文档数：{detail.documents?.length || 0}</p>
            <form onSubmit={(e) => { e.preventDefault(); const title = e.target.title.value; const content = e.target.content.value; if (content) addDoc(detail.id, title, content) }} className="bg-slate-900/60 rounded-xl p-4 border border-slate-700 space-y-2">
              <input name="title" className="input" placeholder="文档标题" />
              <textarea name="content" className="input h-24" placeholder="文档内容（自动切片）" required />
              <button className="w-full bg-gradient-to-r from-blue-600 to-purple-600 text-white px-4 py-2.5 rounded-xl">添加文档</button>
            </form>
            <ul className="space-y-2">
              {(detail.documents || []).map((doc) => (
                <li key={doc.id} className="bg-slate-900/60 rounded-xl p-3 border border-slate-700 text-sm">
                  <div className="flex justify-between text-slate-300">
                    <span>{doc.title || '未命名'}</span>
                    <span className="text-slate-500">{doc.segments} 切片 · {doc.status}</span>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        </Modal>
      )}
    </Panel>
  )
}

const Panel = ({ loading, onRefresh, onAdd, addLabel, children }) => (
  <div>
    <div className="flex items-center justify-end gap-2 mb-6">
      {onAdd && (
        <button onClick={onAdd} className="flex items-center gap-1.5 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-4 py-2 rounded-xl text-sm font-medium">
          <Plus className="w-4 h-4" /> {addLabel}
        </button>
      )}
      <button onClick={onRefresh} className="p-2 text-slate-400 hover:text-blue-400 hover:bg-slate-700 rounded-lg transition-colors" title="刷新">
        <RefreshCw className="w-4 h-4" />
      </button>
    </div>
    {loading ? <div className="text-center text-slate-400 py-16">加载中...</div> : children}
  </div>
)

const Card = ({ title, subtitle, onDelete, onClick, children }) => (
  <div onClick={onClick} className={`bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6 hover:border-blue-500/50 transition-all ${onClick ? 'cursor-pointer' : ''}`}>
    <div className="flex items-start justify-between mb-3">
      <div>
        <h3 className="font-bold text-white">{title}</h3>
        {subtitle && <span className="text-xs text-slate-400">{subtitle}</span>}
      </div>
      {onDelete && (
        <button onClick={(e) => { e.stopPropagation(); onDelete() }} className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg">
          <Trash2 className="w-4 h-4" />
        </button>
      )}
    </div>
    {children}
  </div>
)

const Modal = ({ title, onClose, large, children }) => (
  <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4" onClick={onClose}>
    <div onClick={(e) => e.stopPropagation()} className={`bg-slate-800 rounded-2xl p-8 w-full ${large ? 'max-w-3xl' : 'max-w-lg'} border border-slate-700 shadow-2xl max-h-[90vh] overflow-y-auto`}>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-white">{title}</h2>
        <button onClick={onClose} className="text-slate-400 hover:text-white text-2xl leading-none">×</button>
      </div>
      {children}
    </div>
  </div>
)

const Stat = ({ label, value }) => (
  <div className="bg-slate-800 rounded-2xl border border-slate-700 p-5">
    <div className="text-slate-400 text-sm mb-1">{label}</div>
    <div className="text-2xl font-bold text-white">{value}</div>
  </div>
)

const Metric = ({ label, value }) => (
  <div>
    <div className="text-slate-400 text-xs">{label}</div>
    <div className="text-slate-200">{value ?? '—'}</div>
  </div>
)

export default LLMOps
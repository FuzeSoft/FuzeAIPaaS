import React, { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../auth'
import { usePolling, POLL_INTERVAL_MS } from '../utils/usePolling'
import { Bell, Plus, Trash2, Power, PowerOff, Clock, ShieldOff, AlertTriangle, CheckCircle2, XCircle } from 'lucide-react'

const SEVERITIES = ['info', 'warning', 'critical']

const severityStyle = (s) => {
  switch (s) {
    case 'critical': return 'bg-red-500/20 text-red-400 border-red-500/40'
    case 'warning': return 'bg-amber-500/20 text-amber-400 border-amber-500/40'
    default: return 'bg-sky-500/20 text-sky-400 border-sky-500/40'
  }
}

const Alerts = () => {
  const [rules, setRules] = useState([])
  const [active, setActive] = useState([])
  const [silences, setSilences] = useState([])
  const [loading, setLoading] = useState(true)
  const [tab, setTab] = useState('rules') 
  const [drawer, setDrawer] = useState(null) 
  const [error, setError] = useState(null)

  const loadAllSilent = useCallback(async () => {
    const [r, a, s] = await Promise.all([
      apiFetch('/api/v1/alerts/rules').then((x) => x.json()).catch(() => []),
      apiFetch('/api/v1/alerts/active').then((x) => x.json()).catch(() => []),
      apiFetch('/api/v1/alerts/silences').then((x) => x.json()).catch(() => []),
    ])
    setRules(Array.isArray(r) ? r : [])
    setActive(Array.isArray(a) ? a : [])
    setSilences(Array.isArray(s) ? s : [])
  }, [])

  const loadAll = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      await loadAllSilent()
    } catch (e) {
      setError('加载告警数据失败：' + e.message)
    } finally {
      setLoading(false)
    }
  }, [loadAllSilent])

  usePolling(
    () => (tab === 'active' ? loadActiveOnly() : loadAllSilent()),
    { intervalMs: POLL_INTERVAL_MS, immediate: false }
  )

  useEffect(() => { loadAll() }, [loadAll])

  const loadActiveOnly = async () => {
    try {
      const a = await apiFetch('/api/v1/alerts/active').then((x) => x.json())
      setActive(Array.isArray(a) ? a : [])
    } catch (e) {  }
  }

  const toggleRule = async (rule) => {
    try {
      await apiFetch(`/api/v1/alerts/rules/${rule.id}/toggle`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !rule.enabled }),
      })
      loadAll()
    } catch (e) { setError('切换规则失败：' + e.message) }
  }

  const deleteRule = async (id) => {
    if (!confirm('确认删除该告警规则？')) return
    try {
      await apiFetch(`/api/v1/alerts/rules/${id}`, { method: 'DELETE' })
      loadAll()
    } catch (e) { setError('删除失败：' + e.message) }
  }

  const deleteSilence = async (id) => {
    if (!confirm('确认取消该静默？')) return
    try {
      await apiFetch(`/api/v1/alerts/silences/${id}`, { method: 'DELETE' })
      loadAll()
    } catch (e) { setError('删除静默失败：' + e.message) }
  }

  const firingCount = active.filter((a) => a.state === 'firing').length

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2">告警中心</h1>
            <p className="text-slate-400">规则由平台管理（API+DB），评估交给 Prometheus；活跃告警实时查询</p>
          </div>
          <div className="flex items-center gap-3">
            {firingCount > 0 ? (
              <span className="flex items-center gap-2 bg-red-500/20 text-red-400 border border-red-500/40 px-4 py-2 rounded-xl text-sm font-medium">
                <AlertTriangle className="w-4 h-4" /> {firingCount} 条告警触发中
              </span>
            ) : (
              <span className="flex items-center gap-2 bg-green-500/20 text-green-400 border border-green-500/40 px-4 py-2 rounded-xl text-sm font-medium">
                <CheckCircle2 className="w-4 h-4" /> 当前无触发告警
              </span>
            )}
            <button
              onClick={() => setDrawer('create')}
              className="flex items-center gap-2 bg-gradient-to-r from-blue-500 to-purple-600 hover:from-blue-600 hover:to-purple-700 text-white px-5 py-2.5 rounded-xl font-medium transition-all shadow-lg"
            >
              <Plus className="w-4 h-4" /> 新建规则
            </button>
          </div>
        </div>

        {error && (
          <div className="mb-4 bg-red-500/10 border border-red-500/30 text-red-300 px-4 py-3 rounded-xl text-sm">{error}</div>
        )}

        <div className="flex gap-2 mb-6 border-b border-slate-700">
          {[
            { key: 'rules', label: '告警规则', icon: Bell },
            { key: 'active', label: '活跃告警', icon: AlertTriangle },
            { key: 'silences', label: '静默', icon: ShieldOff },
          ].map((t) => {
            const Icon = t.icon
            return (
              <button
                key={t.key}
                onClick={() => setTab(t.key)}
                className={`flex items-center gap-2 px-4 py-3 text-sm font-medium border-b-2 -mb-px transition-colors ${
                  tab === t.key ? 'border-blue-500 text-white' : 'border-transparent text-slate-400 hover:text-white'
                }`}
              >
                <Icon className="w-4 h-4" /> {t.label}
                {t.key === 'rules' && rules.length > 0 && <span className="text-xs bg-slate-700 text-slate-300 rounded-full px-2 py-0.5">{rules.length}</span>}
                {t.key === 'active' && firingCount > 0 && <span className="text-xs bg-red-500/30 text-red-300 rounded-full px-2 py-0.5">{firingCount}</span>}
              </button>
            )
          })}
        </div>

        {loading && <div className="text-center text-slate-500 py-8">加载中…</div>}

        {!loading && tab === 'rules' && (
          <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-slate-900/60 text-slate-400">
                <tr>
                  <th className="text-left px-4 py-3 font-medium">名称</th>
                  <th className="text-left px-4 py-3 font-medium">表达式</th>
                  <th className="text-left px-4 py-3 font-medium">级别</th>
                  <th className="text-left px-4 py-3 font-medium">For</th>
                  <th className="text-left px-4 py-3 font-medium">状态</th>
                  <th className="text-right px-4 py-3 font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {rules.length === 0 && (
                  <tr><td colSpan={6} className="text-center text-slate-500 py-8">尚无告警规则，点击右上角新建</td></tr>
                )}
                {rules.map((r) => (
                  <tr key={r.id} className="border-t border-slate-700/60 hover:bg-slate-700/30">
                    <td className="px-4 py-3 text-white font-medium">{r.name}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-300 max-w-md truncate" title={r.expr}>{r.expr}</td>
                    <td className="px-4 py-3">
                      <span className={`text-xs px-2 py-0.5 rounded-full border ${severityStyle(r.severity)}`}>{r.severity}</span>
                    </td>
                    <td className="px-4 py-3 text-slate-400">{r.for || '-'}</td>
                    <td className="px-4 py-3">
                      {r.enabled
                        ? <span className="text-xs text-green-400 flex items-center gap-1"><CheckCircle2 className="w-3.5 h-3.5" />启用</span>
                        : <span className="text-xs text-slate-500 flex items-center gap-1"><XCircle className="w-3.5 h-3.5" />禁用</span>}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        <button onClick={() => toggleRule(r)} title={r.enabled ? '禁用' : '启用'} className="p-2 rounded-lg bg-slate-700/60 hover:bg-slate-600 text-slate-300">
                          {r.enabled ? <PowerOff className="w-4 h-4" /> : <Power className="w-4 h-4" />}
                        </button>
                        <button onClick={() => deleteRule(r.id)} title="删除" className="p-2 rounded-lg bg-red-500/20 hover:bg-red-500/40 text-red-300">
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {!loading && tab === 'active' && (
          <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-slate-900/60 text-slate-400">
                <tr>
                  <th className="text-left px-4 py-3 font-medium">规则</th>
                  <th className="text-left px-4 py-3 font-medium">级别</th>
                  <th className="text-left px-4 py-3 font-medium">状态</th>
                  <th className="text-left px-4 py-3 font-medium">摘要</th>
                  <th className="text-left px-4 py-3 font-medium">触发时间</th>
                </tr>
              </thead>
              <tbody>
                {active.length === 0 && (
                  <tr><td colSpan={5} className="text-center text-slate-500 py-8">当前无活跃告警</td></tr>
                )}
                {active.map((a) => (
                  <tr key={a.fingerprint} className="border-t border-slate-700/60">
                    <td className="px-4 py-3 text-white font-medium">{a.rule_name || a.labels?.alertname}</td>
                    <td className="px-4 py-3"><span className={`text-xs px-2 py-0.5 rounded-full border ${severityStyle(a.severity)}`}>{a.severity || '-'}</span></td>
                    <td className="px-4 py-3">
                      <span className={`text-xs px-2 py-0.5 rounded-full border ${a.state === 'firing' ? 'bg-red-500/20 text-red-400 border-red-500/40' : 'bg-slate-600/20 text-slate-300 border-slate-500/40'}`}>{a.state}</span>
                    </td>
                    <td className="px-4 py-3 text-slate-300 max-w-md">{a.annotations?.summary || a.annotations?.description || '-'}</td>
                    <td className="px-4 py-3 text-slate-400 text-xs">{a.active_at ? new Date(a.active_at).toLocaleString() : '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {!loading && tab === 'silences' && (
          <div className="space-y-4">
            <SilenceForm onCreated={loadAll} />
            <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl overflow-hidden">
              <table className="w-full text-sm">
                <thead className="bg-slate-900/60 text-slate-400">
                  <tr>
                    <th className="text-left px-4 py-3 font-medium">备注</th>
                    <th className="text-left px-4 py-3 font-medium">规则</th>
                    <th className="text-left px-4 py-3 font-medium">起止</th>
                    <th className="text-right px-4 py-3 font-medium">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {silences.length === 0 && (
                    <tr><td colSpan={4} className="text-center text-slate-500 py-8">尚无静默</td></tr>
                  )}
                  {silences.map((s) => (
                    <tr key={s.id} className="border-t border-slate-700/60">
                      <td className="px-4 py-3 text-slate-200">{s.comment || '-'}</td>
                      <td className="px-4 py-3 text-slate-400">{s.rule_id || '全租户'}</td>
                      <td className="px-4 py-3 text-slate-400 text-xs">{new Date(s.starts_at).toLocaleString()} ~ {new Date(s.ends_at).toLocaleString()}</td>
                      <td className="px-4 py-3">
                        <div className="flex justify-end">
                          <button onClick={() => deleteSilence(s.id)} className="p-2 rounded-lg bg-red-500/20 hover:bg-red-500/40 text-red-300">
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>

      {drawer && (
        <RuleDrawer
          initial={drawer === 'create' ? null : drawer}
          onClose={() => setDrawer(null)}
          onSaved={() => { setDrawer(null); loadAll() }}
        />
      )}
    </div>
  )
}

const RuleDrawer = ({ initial, onClose, onSaved }) => {
  const [form, setForm] = useState({
    name: initial?.name || '',
    expr: initial?.expr || '',
    for: initial?.for || '5m',
    severity: initial?.severity || 'warning',
    summary: initial?.summary || '',
    description: initial?.description || '',
    enabled: initial?.enabled ?? true,
  })
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState(null)

  const submit = async () => {
    if (!form.name || !form.expr) { setErr('名称与表达式必填'); return }
    setSaving(true)
    setErr(null)
    try {
      const method = initial ? 'PUT' : 'POST'
      const url = initial ? `/api/v1/alerts/rules/${initial.id}` : '/api/v1/alerts/rules'
      const res = await apiFetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || `HTTP ${res.status}`)
      }
      onSaved()
    } catch (e) {
      setErr('保存失败：' + e.message)
    } finally {
      setSaving(false)
    }
  }

  const field = (k) => ({ value: form[k], onChange: (e) => setForm({ ...form, [k]: e.target.value }) })

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex justify-end" onClick={onClose}>
      <div className="w-full max-w-lg bg-slate-900 border-l border-slate-700 h-full overflow-y-auto p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-xl font-bold text-white mb-6">{initial ? '编辑告警规则' : '新建告警规则'}</h2>
        {err && <div className="mb-4 bg-red-500/10 border border-red-500/30 text-red-300 px-4 py-3 rounded-xl text-sm">{err}</div>}
        <div className="space-y-4">
          <div>
            <label className="block text-sm text-slate-400 mb-1">名称</label>
            <input {...field('name')} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white" placeholder="GPUIdle" />
          </div>
          <div>
            <label className="block text-sm text-slate-400 mb-1">PromQL 表达式</label>
            <textarea {...field('expr')} rows={3} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white font-mono text-xs" placeholder="avg(fuze_gpu_utilization_percent) < 5" />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-slate-400 mb-1">持续 (for)</label>
              <input {...field('for')} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white font-mono text-xs" placeholder="5m" />
            </div>
            <div>
              <label className="block text-sm text-slate-400 mb-1">级别</label>
              <select {...field('severity')} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white">
                {SEVERITIES.map((s) => <option key={s} value={s}>{s}</option>)}
              </select>
            </div>
          </div>
          <div>
            <label className="block text-sm text-slate-400 mb-1">摘要</label>
            <input {...field('summary')} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white" placeholder="GPU 平均利用率过低" />
          </div>
          <div>
            <label className="block text-sm text-slate-400 mb-1">描述</label>
            <textarea {...field('description')} rows={2} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white text-sm" />
          </div>
          <label className="flex items-center gap-2 text-sm text-slate-300">
            <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />
            启用该规则
          </label>
        </div>
        <div className="flex justify-end gap-3 mt-8">
          <button onClick={onClose} className="px-4 py-2 rounded-lg bg-slate-700 hover:bg-slate-600 text-white text-sm">取消</button>
          <button onClick={submit} disabled={saving} className="px-4 py-2 rounded-lg bg-gradient-to-r from-blue-500 to-purple-600 hover:from-blue-600 hover:to-purple-700 text-white text-sm disabled:opacity-50">
            {saving ? '保存中…' : '保存'}
          </button>
        </div>
      </div>
    </div>
  )
}

const SilenceForm = ({ onCreated }) => {
  const [comment, setComment] = useState('')
  const [durationMin, setDurationMin] = useState(60)
  const [saving, setSaving] = useState(false)

  const submit = async () => {
    const endsAt = Date.now() + durationMin * 60 * 1000
    setSaving(true)
    try {
      await apiFetch('/api/v1/alerts/silences', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ comment, ends_at: endsAt }),
      })
      setComment('')
      onCreated()
    } catch (e) { alert('创建静默失败：' + e.message) } finally { setSaving(false) }
  }

  return (
    <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-4 flex flex-wrap items-end gap-3">
      <div className="flex-1 min-w-[200px]">
        <label className="block text-xs text-slate-400 mb-1">静默备注</label>
        <input value={comment} onChange={(e) => setComment(e.target.value)} placeholder="临时维护窗口，屏蔽全部告警" className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white text-sm" />
      </div>
      <div>
        <label className="block text-xs text-slate-400 mb-1">时长(分钟)</label>
        <input type="number" value={durationMin} onChange={(e) => setDurationMin(Number(e.target.value))} className="w-28 bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white text-sm" />
      </div>
      <button onClick={submit} disabled={saving} className="flex items-center gap-2 bg-gradient-to-r from-blue-500 to-purple-600 hover:from-blue-600 hover:to-purple-700 text-white px-4 py-2 rounded-xl text-sm disabled:opacity-50">
        <Clock className="w-4 h-4" /> 创建静默
      </button>
    </div>
  )
}

export default Alerts
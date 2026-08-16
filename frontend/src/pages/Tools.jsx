import React, { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../auth'
import { Wrench, Plus, Trash2, Globe, ShieldAlert, ShieldCheck, CheckCircle2 } from 'lucide-react'

const Tools = () => {
  const [tools, setTools] = useState([])
  const [loading, setLoading] = useState(true)
  const [drawer, setDrawer] = useState(false)
  const [error, setError] = useState(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const d = await apiFetch('/api/v1/tools').then((x) => x.json())
      setTools(Array.isArray(d.tools) ? d.tools : [])
    } catch (e) {
      setError('加载工具失败：' + e.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const deleteTool = async (id) => {
    if (!confirm('确认删除该工具？引用它的 Agent 运行将失败。')) return
    try {
      const res = await apiFetch(`/api/v1/tools/${id}`, { method: 'DELETE' })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || `HTTP ${res.status}`)
      }
      load()
    } catch (e) { setError('删除失败：' + e.message) }
  }

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2">工具注册表</h1>
            <p className="text-slate-400">供 Agent 编排的 tool_call 节点调用；HTTP 工具经出网代理执行并做 SSRF 防护</p>
          </div>
          <button
            onClick={() => setDrawer(true)}
            className="flex items-center gap-2 bg-gradient-to-r from-blue-500 to-purple-600 hover:from-blue-600 hover:to-purple-700 text-white px-5 py-2.5 rounded-xl font-medium transition-all shadow-lg"
          >
            <Plus className="w-4 h-4" /> 新建工具
          </button>
        </div>

        {error && (
          <div className="mb-4 bg-red-500/10 border border-red-500/30 text-red-300 px-4 py-3 rounded-xl text-sm">{error}</div>
        )}

        {loading ? (
          <div className="text-center text-slate-500 py-8">加载中…</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {tools.length === 0 && (
              <div className="col-span-full text-center text-slate-500 py-8 bg-slate-800 rounded-2xl border border-slate-700">
                尚无工具，点击右上角新建一个 HTTP 工具
              </div>
            )}
            {tools.map((t) => (
              <div key={t.id} className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-5 hover:border-slate-600 transition-colors">
                <div className="flex items-start justify-between mb-3">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-xl flex items-center justify-center">
                      <Wrench className="w-5 h-5 text-white" />
                    </div>
                    <div>
                      <h3 className="text-white font-semibold">{t.name}</h3>
                      <p className="text-xs text-slate-400">{t.id}</p>
                    </div>
                  </div>
                  <button onClick={() => deleteTool(t.id)} title="删除" className="p-2 rounded-lg bg-red-500/20 hover:bg-red-500/40 text-red-300">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
                {t.description && <p className="text-sm text-slate-400 mb-3">{t.description}</p>}
                <div className="space-y-2 text-xs">
                  <div className="flex items-center gap-2 text-slate-300">
                    <span className="px-2 py-0.5 rounded-full bg-slate-700 text-slate-200">{t.kind || 'http'}</span>
                    {t.sensitive
                      ? <span className="flex items-center gap-1 text-amber-400"><ShieldAlert className="w-3.5 h-3.5" />敏感</span>
                      : <span className="flex items-center gap-1 text-slate-400"><ShieldCheck className="w-3.5 h-3.5" />普通</span>}
                  </div>
                  {t.http && (
                    <div className="flex items-center gap-2 text-slate-400 font-mono break-all">
                      <Globe className="w-3.5 h-3.5 shrink-0" />
                      <span>{t.http.method || 'POST'} {t.http.url}</span>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {drawer && (
        <ToolDrawer
          onClose={() => setDrawer(false)}
          onSaved={() => { setDrawer(false); load() }}
        />
      )}
    </div>
  )
}

const ToolDrawer = ({ onClose, onSaved }) => {
  const [form, setForm] = useState({
    name: '', description: '', kind: 'http',
    url: '', method: 'POST', sensitive: false,
  })
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState(null)

  const submit = async () => {
    if (!form.name || !form.url) { setErr('工具名与 URL 必填'); return }
    setSaving(true)
    setErr(null)
    try {
      const res = await apiFetch('/api/v1/tools', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: form.name,
          description: form.description,
          kind: form.kind,
          http: { url: form.url, method: form.method },
          sensitive: form.sensitive,
        }),
      })
      const d = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(d.error || `HTTP ${res.status}`)
      onSaved()
    } catch (e) {
      setErr('保存失败：' + e.message)
    } finally {
      setSaving(false)
    }
  }

  const set = (k) => ({ value: form[k], onChange: (e) => setForm({ ...form, [k]: e.target.value }) })
  const setCheck = (k) => ({ checked: form[k], onChange: (e) => setForm({ ...form, [k]: e.target.checked }) })

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex justify-end" onClick={onClose}>
      <div className="w-full max-w-lg bg-slate-900 border-l border-slate-700 h-full overflow-y-auto p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-xl font-bold text-white mb-6">新建 HTTP 工具</h2>
        {err && <div className="mb-4 bg-red-500/10 border border-red-500/30 text-red-300 px-4 py-3 rounded-xl text-sm">{err}</div>}
        <div className="space-y-4">
          <div>
            <label className="block text-sm text-slate-400 mb-1">工具名（Agent 中引用）</label>
            <input {...set('name')} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white" placeholder="search_weather" />
          </div>
          <div>
            <label className="block text-sm text-slate-400 mb-1">描述</label>
            <input {...set('description')} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white" />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-slate-400 mb-1">方法</label>
              <select {...set('method')} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white">
                <option value="POST">POST</option>
                <option value="GET">GET</option>
                <option value="PUT">PUT</option>
                <option value="DELETE">DELETE</option>
              </select>
            </div>
            <div>
              <label className="block text-sm text-slate-400 mb-1">类型</label>
              <input value="http" disabled className="w-full bg-slate-800/50 border border-slate-700 rounded-lg px-3 py-2 text-slate-400" />
            </div>
          </div>
          <div>
            <label className="block text-sm text-slate-400 mb-1">URL（仅 http/https）</label>
            <input {...set('url')} className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-white font-mono text-xs" placeholder="https://api.example.com/tool" />
          </div>
          <label className="flex items-center gap-2 text-sm text-slate-300">
            <input type="checkbox" {...setCheck('sensitive')} />
            敏感工具（Agent 运行中拒绝自动执行，需人工确认）
          </label>
          <div className="flex items-center gap-2 text-xs text-slate-500">
            <CheckCircle2 className="w-4 h-4 text-green-400" />
            执行时由出网代理调用，并做私网地址 SSRF 防护
          </div>
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

export default Tools
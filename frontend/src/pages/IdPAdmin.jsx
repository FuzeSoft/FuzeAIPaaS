import React, { useState, useEffect } from 'react'
import { apiJson, isPlatformAdmin } from '../auth'

const inputCls =
  'w-full px-3 py-2.5 rounded-lg bg-slate-800/60 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500'

function Field({ label, children }) {
  return (
    <label className="block mb-3">
      <span className="block text-xs font-medium text-slate-400 mb-1">{label}</span>
      {children}
    </label>
  )
}

function emptyForm() {
  return {
    provider_id: '',
    type: 'oidc',
    name: '',
    enabled: true,
    issuer: '',
    client_id: '',
    client_secret: '',
    redirect_uri: '',
    scopes: '',
    ldap_addr: '',
    ldap_use_tls: false,
    ldap_skip_verify: false,
    ldap_user_dn_format: '',
    default_role: 'member',
    admin_groups: '',
    admin_role: 'admin',
    default_tenant: '',
  }
}

export default function IdPAdmin() {
  const [idps, setIdps] = useState([])
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')
  const [editing, setEditing] = useState(null) 
  const [form, setForm] = useState(emptyForm())
  const [saving, setSaving] = useState(false)

  const load = async () => {
    try {
      const d = await apiJson('/sso/idps')
      setIdps(d.idps || [])
    } catch (e) {
      setError(e.message)
    }
  }
  useEffect(() => { load() }, [])

  const openNew = () => {
    setError(''); setMsg('')
    setForm(emptyForm())
    setEditing('new')
  }

  const openEdit = (p) => {
    setError(''); setMsg('')
    setForm({
      provider_id: p.provider_id,
      type: p.type,
      name: p.name,
      enabled: p.enabled,
      issuer: p.issuer || '',
      client_id: p.client_id || '',
      client_secret: '', 
      redirect_uri: p.redirect_uri || '',
      scopes: p.scopes || '',
      ldap_addr: p.ldap_addr || '',
      ldap_use_tls: p.ldap_use_tls || false,
      ldap_skip_verify: p.ldap_skip_verify || false,
      ldap_user_dn_format: p.ldap_user_dn_format || '',
      default_role: p.default_role || 'member',
      admin_groups: (p.admin_groups || []).join(','),
      admin_role: p.admin_role || 'admin',
      default_tenant: p.default_tenant || '',
    })
    setEditing(p.provider_id)
  }

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))

  const save = async (e) => {
    e.preventDefault()
    setError(''); setMsg(''); setSaving(true)
    const payload = { ...form }
    if (payload.admin_groups) payload.admin_groups = payload.admin_groups.split(',').map((s) => s.trim()).filter(Boolean)
    if (!payload.client_secret) delete payload.client_secret 
    try {
      if (editing === 'new') {
        await apiJson('/sso/idps', { method: 'POST', body: payload })
        setMsg('IdP 已创建')
      } else {
        await apiJson(`/sso/idps/${encodeURIComponent(editing)}`, { method: 'PUT', body: payload })
        setMsg('IdP 已更新')
      }
      setEditing(null)
      load()
    } catch (e) {
      setError(e.message)
    } finally {
      setSaving(false)
    }
  }

  const remove = async (pid) => {
    if (!window.confirm(`确定删除 IdP「${pid}」？`)) return
    setError(''); setMsg('')
    try {
      await apiJson(`/sso/idps/${encodeURIComponent(pid)}`, { method: 'DELETE' })
      setMsg('IdP 已删除')
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  const [testResult, setTestResult] = useState(null) 
  const testConn = async (pid) => {
    setTestResult({ pid, loading: true })
    try {
      const d = await apiJson(`/sso/idps/${encodeURIComponent(pid)}/test`, { method: 'POST' })
      setTestResult({ pid, ok: d.ok, detail: d.detail })
    } catch (e) {
      setTestResult({ pid, ok: false, detail: e.message })
    }
  }

  const fetchMetadata = async () => {
    setError('')
    const issuer = form.issuer.trim().replace(/\/$/, '')
    if (!issuer) {
      setError('请先填写 Issuer')
      return
    }
    if (!/^https?:\/\
      setError('Issuer 需以 http(s):// 开头')
      return
    }
    try {
      const resp = await fetch(issuer + '/.well-known/openid-configuration')
      if (!resp.ok) throw new Error(`discovery 返回 ${resp.status}`)
      const doc = await resp.json()
      set('issuer', issuer)
      if (doc.authorization_endpoint && !form.redirect_uri) {
        
      }
      if (doc.scopes_supported && !form.scopes) {
        set('scopes', doc.scopes_supported.filter((s) => ['openid', 'profile', 'email'].includes(s)).join(','))
      }
      setMsg('已从 issuer 拉取元数据（scopes 已尝试自动填充）')
    } catch (e) {
      setError('拉取元数据失败：' + e.message + '（可能因浏览器跨域限制，请在服务端网络可达时重试）')
    }
  }

  if (!isPlatformAdmin()) {
    return (
      <div className="max-w-4xl mx-auto">
        <h2 className="text-xl font-bold text-white mb-4">身份提供方 (IdP) 管理</h2>
        <div className="text-sm text-red-400 bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2">
          仅平台管理员可访问此页面。
        </div>
      </div>
    )
  }

  return (
    <div className="max-w-4xl mx-auto">
      <div className="flex items-center justify-between mb-4">
        <div>
          <h2 className="text-xl font-bold text-white">身份提供方 (IdP) 管理</h2>
          <p className="text-sm text-slate-400 mt-1">配置 OIDC / LDAP / SAML 单点登录与目录接入</p>
        </div>
        {editing === null && (
          <button onClick={openNew} className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium">
            + 新建 IdP
          </button>
        )}
      </div>

      {error && <div className="text-xs text-red-400 bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2 mb-3">{error}</div>}
      {msg && <div className="text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/30 rounded-lg px-3 py-2 mb-3">{msg}</div>}

      {editing === null && (
        idps.length === 0 ? (
          <div className="bg-slate-900/70 border border-slate-800 rounded-2xl p-8 text-center text-sm text-slate-500">
            暂无 IdP 配置。点击右上角「新建 IdP」开始接入企业身份源。
          </div>
        ) : (
          <div className="space-y-3">
            {idps.map((p) => (
              <div key={p.provider_id} className="bg-slate-900/70 border border-slate-800 rounded-2xl p-5 flex items-center justify-between">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-white font-medium">{p.name || p.provider_id}</span>
                    <span className="text-xs px-2 py-0.5 rounded-full bg-slate-800 text-slate-300">{p.type}</span>
                    <span className={'text-xs px-2 py-0.5 rounded-full ' + (p.enabled ? 'bg-emerald-500/20 text-emerald-300' : 'bg-slate-700 text-slate-400')}>
                      {p.enabled ? '启用' : '禁用'}
                    </span>
                  </div>
                  <div className="text-xs text-slate-500 mt-1 font-mono">{p.provider_id}</div>
                </div>
                <div className="flex gap-2">
                  <button onClick={() => openEdit(p)} className="text-blue-400 hover:underline text-sm">编辑</button>
                  <button onClick={() => remove(p.provider_id)} className="text-red-400 hover:underline text-sm">删除</button>
                  <button onClick={() => testConn(p.provider_id)} className="text-emerald-400 hover:underline text-sm">测试连通性</button>
                </div>
                {testResult && testResult.pid === p.provider_id && (
                  <div className={'text-xs mt-2 ' + (testResult.loading ? 'text-slate-400' : testResult.ok ? 'text-emerald-400' : 'text-red-400')}>
                    {testResult.loading ? '探测中…' : (testResult.ok ? '✓ ' : '✗ ') + (testResult.detail || '')}
                  </div>
                )}
              </div>
            ))}
          </div>
        )
      )}

      {editing !== null && (
        <form onSubmit={save} className="bg-slate-900/70 border border-slate-800 rounded-2xl p-6 space-y-2">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
            <Field label="Provider ID（唯一，不可改）">
              <input value={form.provider_id} onChange={(e) => set('provider_id', e.target.value)} disabled={editing !== 'new'} className={inputCls} required />
            </Field>
            <Field label="类型">
              <select value={form.type} onChange={(e) => set('type', e.target.value)} className={inputCls}>
                <option value="oidc">OIDC</option>
                <option value="ldap">LDAP</option>
                <option value="saml">SAML</option>
              </select>
            </Field>
            <Field label="显示名称">
              <input value={form.name} onChange={(e) => set('name', e.target.value)} className={inputCls} />
            </Field>
            <Field label="启用">
              <input type="checkbox" checked={form.enabled} onChange={(e) => set('enabled', e.target.checked)} className="mt-2 w-4 h-4" />
            </Field>
          </div>

          {form.type === 'oidc' && (
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
              <Field label="Issuer">
                <input value={form.issuer} onChange={(e) => set('issuer', e.target.value)} className={inputCls} placeholder="https://idp.example.com" />
              </Field>
              <div className="flex items-end">
                <button type="button" onClick={fetchMetadata} className="px-3 py-2.5 rounded-lg bg-slate-700 hover:bg-slate-600 text-white text-sm">
                  从 Issuer 拉取元数据
                </button>
              </div>
              <Field label="Client ID">
                <input value={form.client_id} onChange={(e) => set('client_id', e.target.value)} className={inputCls} />
              </Field>
              <Field label="Client Secret（留空=不修改）">
                <input type="password" value={form.client_secret} onChange={(e) => set('client_secret', e.target.value)} className={inputCls} />
              </Field>
              <Field label="Redirect URI">
                <input value={form.redirect_uri} onChange={(e) => set('redirect_uri', e.target.value)} className={inputCls} />
              </Field>
              <Field label="Scopes（逗号分隔）">
                <input value={form.scopes} onChange={(e) => set('scopes', e.target.value)} className={inputCls} placeholder="openid,profile,email" />
              </Field>
            </div>
          )}

          {form.type === 'ldap' && (
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
              <Field label="LDAP 地址">
                <input value={form.ldap_addr} onChange={(e) => set('ldap_addr', e.target.value)} className={inputCls} placeholder="ldaps://ldap.example.com:636" />
              </Field>
              <Field label="用户 DN 格式">
                <input value={form.ldap_user_dn_format} onChange={(e) => set('ldap_user_dn_format', e.target.value)} className={inputCls} placeholder="uid=%s,ou=users,dc=example,dc=com" />
              </Field>
              <Field label="使用 TLS">
                <input type="checkbox" checked={form.ldap_use_tls} onChange={(e) => set('ldap_use_tls', e.target.checked)} className="mt-2 w-4 h-4" />
              </Field>
              <Field label="跳过证书校验">
                <input type="checkbox" checked={form.ldap_skip_verify} onChange={(e) => set('ldap_skip_verify', e.target.checked)} className="mt-2 w-4 h-4" />
              </Field>
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
            <Field label="默认角色">
              <select value={form.default_role} onChange={(e) => set('default_role', e.target.value)} className={inputCls}>
                <option value="member">member</option>
                <option value="admin">admin</option>
                <option value="viewer">viewer</option>
              </select>
            </Field>
            <Field label="默认租户">
              <input value={form.default_tenant} onChange={(e) => set('default_tenant', e.target.value)} className={inputCls} />
            </Field>
            <Field label="管理员 Groups（逗号分隔）">
              <input value={form.admin_groups} onChange={(e) => set('admin_groups', e.target.value)} className={inputCls} placeholder="admins,platform-admins" />
            </Field>
            <Field label="管理员角色">
              <input value={form.admin_role} onChange={(e) => set('admin_role', e.target.value)} className={inputCls} />
            </Field>
          </div>

          <div className="flex gap-2 pt-2">
            <button type="submit" disabled={saving} className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium disabled:opacity-60">
              {saving ? '保存中…' : '保存'}
            </button>
            <button type="button" onClick={() => setEditing(null)} className="px-4 py-2 rounded-lg bg-slate-700 hover:bg-slate-600 text-white text-sm">
              取消
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
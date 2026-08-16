import React, { useState, useEffect } from 'react'
import { apiJson, getUser } from '../auth'
import { isWebAuthnSupported, parseCreationOptions, attestationToJSON, assertionToJSON, parseRequestOptions } from '../webauthn'

function Section({ title, desc, children }) {
  return (
    <div className="bg-slate-900/70 border border-slate-800 rounded-2xl p-6">
      <div className="mb-4">
        <h3 className="text-base font-semibold text-white">{title}</h3>
        {desc && <p className="text-sm text-slate-400 mt-1">{desc}</p>}
      </div>
      {children}
    </div>
  )
}

function Field({ label, children }) {
  return (
    <label className="block mb-3">
      <span className="block text-xs font-medium text-slate-400 mb-1">{label}</span>
      {children}
    </label>
  )
}

const inputCls =
  'w-full px-3 py-2.5 rounded-lg bg-slate-800/60 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500'

export default function Settings() {
  const [tab, setTab] = useState('mfa')
  const user = getUser() || {}
  return (
    <div className="max-w-4xl mx-auto">
      <h2 className="text-xl font-bold text-white mb-1">账户设置</h2>
      <p className="text-sm text-slate-400 mb-6">管理多因素认证、个人访问令牌等安全配置</p>

      <div className="flex gap-1 mb-6 border-b border-slate-800">
        {[
          { id: 'mfa', label: '多因素认证 (MFA)' },
          { id: 'passkey', label: 'Passkey 密钥' },
          { id: 'pat', label: '访问令牌 (PAT)' },
        ].map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={
              'px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ' +
              (tab === t.id
                ? 'text-blue-400 border-blue-500'
                : 'text-slate-400 border-transparent hover:text-slate-200')
            }
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'mfa' && <MfaPanel />}
      {tab === 'passkey' && <PasskeyPanel />}
      {tab === 'pat' && <PatPanel />}
    </div>
  )
}

function MfaPanel() {
  const user = getUser() || {}
  const enabled = !!user.mfa_enabled
  const [enroll, setEnroll] = useState(null) 
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)

  const startEnroll = async () => {
    setError(''); setMsg(''); setBusy(true)
    try {
      const d = await apiJson('/auth/mfa/enroll', { method: 'POST' })
      setEnroll(d)
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  const confirmEnroll = async (e) => {
    e.preventDefault()
    setError(''); setMsg(''); setBusy(true)
    try {
      await apiJson('/auth/mfa/enroll', { method: 'POST', body: { code } })
      setMsg('MFA 已启用')
      setEnroll(null); setCode('')
      
      const u = await apiJson('/auth/me')
      localStorage.setItem('fuze_user', JSON.stringify(u))
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  const disable = async () => {
    if (!window.confirm('确定要关闭 MFA 吗？关闭后账户安全性将降低。')) return
    setError(''); setMsg(''); setBusy(true)
    try {
      await apiJson('/auth/mfa/disable', { method: 'POST' })
      setMsg('MFA 已关闭')
      const u = await apiJson('/auth/me')
      localStorage.setItem('fuze_user', JSON.stringify(u))
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Section
      title="多因素认证"
      desc="启用 TOTP（如 Google Authenticator）后，登录需额外输入动态验证码。"
    >
      {!enabled && !enroll && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-slate-400">当前状态：<span className="text-amber-400">未启用</span></span>
          <button
            onClick={startEnroll}
            disabled={busy}
            className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium disabled:opacity-60"
          >
            {busy ? '处理中…' : '启用 MFA'}
          </button>
        </div>
      )}

      {enabled && !enroll && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-slate-400">当前状态：<span className="text-emerald-400">已启用</span></span>
          <button
            onClick={disable}
            disabled={busy}
            className="px-4 py-2 rounded-lg bg-red-600/80 hover:bg-red-500 text-white text-sm font-medium disabled:opacity-60"
          >
            {busy ? '处理中…' : '关闭 MFA'}
          </button>
        </div>
      )}

      {enroll && (
        <form onSubmit={confirmEnroll} className="space-y-4">
          <div className="text-sm text-slate-300">
            1. 使用 Authenticator 应用扫描下方链接或手动输入密钥：
          </div>
          <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-3 break-all text-xs text-slate-300">
            <div className="font-mono">{enroll.secret}</div>
            <a className="text-blue-400 hover:underline mt-2 inline-block" href={enroll.otpauth_uri}>
              {enroll.otpauth_uri}
            </a>
          </div>

          <div className="text-sm text-slate-300">2. 输入应用生成的 6 位动态码以确认：</div>
          <Field label="动态验证码">
            <input
              type="text"
              inputMode="numeric"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="123456"
              className={inputCls + ' tracking-widest'}
              required
              autoFocus
            />
          </Field>

          <div className="text-sm text-slate-300">
            3. 请妥善保存以下恢复码（每个仅可使用一次，关闭 MFA 后作废）：
          </div>
          <div className="grid grid-cols-2 gap-2">
            {(enroll.recovery_codes || []).map((c) => (
              <div key={c} className="font-mono text-xs text-slate-300 bg-slate-800/60 border border-slate-700 rounded px-2 py-1">
                {c}
              </div>
            ))}
          </div>

          {error && <div className="text-xs text-red-400 bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2">{error}</div>}
          {msg && <div className="text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/30 rounded-lg px-3 py-2">{msg}</div>}

          <div className="flex gap-2">
            <button type="submit" disabled={busy} className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium disabled:opacity-60">
              {busy ? '验证中…' : '确认启用'}
            </button>
            <button type="button" onClick={() => setEnroll(null)} className="px-4 py-2 rounded-lg bg-slate-700 hover:bg-slate-600 text-white text-sm">
              取消
            </button>
          </div>
        </form>
      )}
    </Section>
  )
}

function PasskeyPanel() {
  const user = getUser() || {}
  const enabled = !!user.passkey_enabled
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)
  const supported = isWebAuthnSupported()

  const refresh = async () => {
    try {
      const u = await apiJson('/auth/me')
      localStorage.setItem('fuze_user', JSON.stringify(u))
    } catch (e) {
      
    }
  }

  const register = async () => {
    setError(''); setMsg(''); setBusy(true)
    try {
      const options = await apiJson('/auth/passkey/register/begin', { method: 'POST' })
      const publicKey = parseCreationOptions(options)
      const cred = await navigator.credentials.create({ publicKey })
      const res = await fetch('/api/v1/auth/passkey/register/finish', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + (localStorage.getItem('fuze_token') || '') },
        body: attestationToJSON(cred),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || '注册失败')
      setMsg('Passkey 已注册并启用')
      await refresh()
    } catch (e) {
      setError(e.message || 'Passkey 注册失败')
    } finally {
      setBusy(false)
    }
  }

  const disable = async () => {
    if (!window.confirm('关闭 Passkey 后无法再用密钥登录（不影响已注册的凭据清除，仅禁用）。')) return
    setError(''); setMsg(''); setBusy(true)
    try {
      await apiJson('/auth/passkey/disable', { method: 'POST' })
      setMsg('Passkey 已禁用')
      await refresh()
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  if (!supported) {
    return (
      <Section title="Passkey 密钥" desc="使用设备上的指纹 / 面容 / 硬件密钥（WebAuthn / FIDO2）免密码登录。">
        <div className="text-sm text-amber-400 bg-amber-500/10 border border-amber-500/30 rounded-lg px-3 py-2">
          当前浏览器不支持 WebAuthn（navigator.credentials 不可用），无法注册或登录 Passkey。
        </div>
      </Section>
    )
  }

  return (
    <Section title="Passkey 密钥" desc="使用设备上的指纹 / 面容 / 硬件密钥（WebAuthn / FIDO2）免密码登录。注册后下次登录可选择「使用 Passkey」。">
      <div className="flex items-center justify-between">
        <span className="text-sm text-slate-400">当前状态：
          <span className={enabled ? 'text-emerald-400' : 'text-amber-400'}>{enabled ? '已启用' : '未启用'}</span>
        </span>
        {!enabled ? (
          <button onClick={register} disabled={busy} className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium disabled:opacity-60">
            {busy ? '注册中…' : '注册 Passkey'}
          </button>
        ) : (
          <button onClick={disable} disabled={busy} className="px-4 py-2 rounded-lg bg-red-600/80 hover:bg-red-500 text-white text-sm font-medium disabled:opacity-60">
            {busy ? '处理中…' : '禁用 Passkey'}
          </button>
        )}
      </div>
      {error && <div className="text-xs text-red-400 bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2 mt-3">{error}</div>}
      {msg && <div className="text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/30 rounded-lg px-3 py-2 mt-3">{msg}</div>}
    </Section>
  )
}

function PatPanel() {
  const [tokens, setTokens] = useState([])
  const [name, setName] = useState('')
  const [ttl, setTtl] = useState('720')
  const [issued, setIssued] = useState(null)
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)

  const load = async () => {
    try {
      const d = await apiJson('/auth/tokens')
      setTokens(d.tokens || [])
    } catch (e) {
      setError(e.message)
    }
  }
  useEffect(() => { load() }, [])

  const create = async (e) => {
    e.preventDefault()
    setError(''); setMsg(''); setBusy(true)
    try {
      const d = await apiJson('/auth/tokens', {
        method: 'POST',
        body: { name: name || undefined, ttl_hours: Number(ttl) || undefined },
      })
      setIssued(d)
      setName(''); setMsg('')
      load()
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  const rotate = async (id) => {
    if (!window.confirm('轮换将废止旧令牌并签发新令牌，确定？')) return
    setError(''); setMsg('')
    try {
      const d = await apiJson(`/auth/tokens/${id}/rotate`, { method: 'POST' })
      setIssued(d)
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  const remove = async (id) => {
    if (!window.confirm('确定吊销该令牌？操作不可撤销。')) return
    setError(''); setMsg('')
    try {
      await apiJson(`/auth/tokens/${id}`, { method: 'DELETE' })
      setMsg('令牌已吊销')
      load()
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <Section
      title="个人访问令牌 (PAT)"
      desc="用于 SDK / CLI / API 调用的长期令牌。建议定期轮换，泄露后立即吊销。"
    >
      {issued && (
        <div className="mb-4 p-3 rounded-lg bg-amber-500/10 border border-amber-500/30">
          <div className="text-xs text-amber-300 mb-1">新令牌已签发（仅显示一次，请立即保存）：</div>
          <div className="font-mono text-sm text-amber-200 break-all">{issued.token}</div>
        </div>
      )}

      <form onSubmit={create} className="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-6">
        <Field label="名称（可选）">
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="my-cli" className={inputCls} />
        </Field>
        <Field label="有效期（小时，0=长期）">
          <input value={ttl} onChange={(e) => setTtl(e.target.value)} type="number" min="0" className={inputCls} />
        </Field>
        <div className="flex items-end">
          <button type="submit" disabled={busy} className="w-full px-4 py-2.5 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium disabled:opacity-60">
            {busy ? '创建中…' : '创建令牌'}
          </button>
        </div>
      </form>

      {error && <div className="text-xs text-red-400 bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2 mb-3">{error}</div>}
      {msg && <div className="text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/30 rounded-lg px-3 py-2 mb-3">{msg}</div>}

      {tokens.length === 0 ? (
        <p className="text-sm text-slate-500">暂无令牌。</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 text-left border-b border-slate-800">
                <th className="py-2 pr-4 font-medium">名称</th>
                <th className="py-2 pr-4 font-medium">前缀</th>
                <th className="py-2 pr-4 font-medium">创建时间</th>
                <th className="py-2 pr-4 font-medium">过期</th>
                <th className="py-2 pr-4 font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {tokens.map((t) => (
                <tr key={t.id} className="border-b border-slate-800/60 text-slate-300">
                  <td className="py-2 pr-4">{t.name || '—'}</td>
                  <td className="py-2 pr-4 font-mono">{t.prefix}…</td>
                  <td className="py-2 pr-4">{t.created_at || '—'}</td>
                  <td className="py-2 pr-4">{t.expires_at || '长期'}</td>
                  <td className="py-2 pr-4 whitespace-nowrap">
                    <button onClick={() => rotate(t.id)} className="text-blue-400 hover:underline mr-3">轮换</button>
                    <button onClick={() => remove(t.id)} className="text-red-400 hover:underline">吊销</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  )
}
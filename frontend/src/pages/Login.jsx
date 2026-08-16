import React, { useState, useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { apiFetch, setUser, getToken, fetchSession, getSession } from '../auth'
import { isWebAuthnSupported, parseRequestOptions, assertionToJSON } from '../webauthn'

export default function Login() {
  const navigate = useNavigate()
  const location = useLocation()
  const [providers, setProviders] = useState([])
  const [method, setMethod] = useState('local') 
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const [mfaToken, setMfaToken] = useState('')
  const [mfaCode, setMfaCode] = useState('')
  const [mfaRecovery, setMfaRecovery] = useState('')

  useEffect(() => {
    const params = new URLSearchParams(location.search)
    
    const mfaTokenParam = params.get('mfa_token')
    if (mfaTokenParam) {
      setMfaToken(mfaTokenParam)
      setUsername(params.get('sso_user') || '')
    } else if (params.get('sso_user')) {
      
      fetchSession(true)
        .then((me) => {
          if (me) {
            setUser(me)
            window.location.href = '/'
          } else {
            
          }
        })
        .catch(() => {  })
      return
    }
    
    if (getToken()) {
      window.location.href = '/'
      return
    }
    getSession()
      .then((me) => {
        if (me) window.location.href = '/'
      })
      .catch(() => {  })
    apiFetch('/api/v1/auth/sso')
      .then((r) => r.json())
      .then((d) => setProviders(d.providers || []))
      .catch(() => setProviders([]))
  }, [location])

  const oidc = providers.find((p) => p.type === 'oidc')

  const submit = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    const url = method === 'ldap' ? '/api/v1/auth/sso/ldap' : '/api/v1/auth/login'
    try {
      const res = await apiFetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      })
      const data = await res.json()
      if (!res.ok) {
        setError(data.error || '登录失败')
        setLoading(false)
        return
      }
      if (data.mfa_required) {
        setMfaToken(data.mfa_token)
        setUsername(data.user ? data.user.username : username)
        setLoading(false)
        return
      }
      finishLogin(data)
    } catch (err) {
      setError('网络错误：' + err.message)
      setLoading(false)
    }
  }

  const verifyMfa = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const body = { mfa_token: mfaToken }
      if (mfaRecovery) body.recovery_code = mfaRecovery
      else body.code = mfaCode
      const res = await apiFetch('/api/v1/auth/mfa/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const data = await res.json()
      if (!res.ok) {
        setError(data.error || '验证失败')
        setLoading(false)
        return
      }
      finishLogin(data)
    } catch (err) {
      setError('网络错误：' + err.message)
      setLoading(false)
    }
  }

  const finishLogin = (data) => {
    
    setUser(data.user || { username })
    window.location.href = '/'
  }

  const verifyPasskeyLogin = async () => {
    if (!mfaToken) return
    setError('')
    setLoading(true)
    try {
      const res = await apiFetch('/api/v1/auth/passkey/login/begin', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mfa_token: mfaToken }),
      })
      const options = await res.json()
      if (!res.ok) {
        setError(options.error || 'Passkey 挑战发起失败')
        setLoading(false)
        return
      }
      const publicKey = parseRequestOptions(options)
      const cred = await navigator.credentials.get({ publicKey })
      
      const body = JSON.parse(assertionToJSON(cred))
      body.mfa_token = mfaToken
      const fres = await fetch('/api/v1/auth/passkey/login/finish', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const data = await fres.json()
      if (!fres.ok) {
        setError(data.error || 'Passkey 验证失败')
        setLoading(false)
        return
      }
      finishLogin(data)
    } catch (err) {
      setError('Passkey 验证失败：' + (err.message || err))
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-slate-950 via-slate-900 to-slate-800 px-4">
      <div className="w-full max-w-md">
        <div className="flex items-center justify-center gap-3 mb-8">
          <div className="w-12 h-12 bg-gradient-to-br from-blue-500 to-purple-600 rounded-xl flex items-center justify-center">
            <svg className="w-7 h-7 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
              <path strokeLinecap="round" strokeLinejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <div>
            <h1 className="text-2xl font-bold text-white">Fuze AI</h1>
            <p className="text-xs text-slate-400">推理优先 · 信创底座</p>
          </div>
        </div>

        <div className="bg-slate-900/70 border border-slate-800 rounded-2xl p-8 shadow-2xl">
          <h2 className="text-lg font-semibold text-white mb-1">登录到平台</h2>
          <p className="text-sm text-slate-400 mb-6">使用企业账号或单点登录进入控制台</p>

          {oidc && (
            <a
              href={oidc.url}
              className="flex items-center justify-center gap-2 w-full mb-4 py-3 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors"
            >
              {oidc.name}
            </a>
          )}

          {oidc && <div className="flex items-center gap-3 my-4 text-xs text-slate-500">
            <div className="h-px flex-1 bg-slate-800" />
            <span>或</span>
            <div className="h-px flex-1 bg-slate-800" />
          </div>}

          <form onSubmit={submit} className="space-y-4">
            <div>
              <label className="block text-xs font-medium text-slate-400 mb-1">用户名</label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="admin / 企业目录账号"
                className="w-full px-3 py-2.5 rounded-lg bg-slate-800/60 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
                required
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-400 mb-1">密码</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                className="w-full px-3 py-2.5 rounded-lg bg-slate-800/60 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
                required
              />
            </div>

            {providers.find((p) => p.type === 'ldap') && (
              <div className="flex items-center gap-2 text-xs text-slate-400">
                <span>登录方式：</span>
                <button type="button" onClick={() => setMethod('local')} className={method === 'local' ? 'text-blue-400 font-medium' : ''}>本地账号</button>
                <span>/</span>
                <button type="button" onClick={() => setMethod('ldap')} className={method === 'ldap' ? 'text-blue-400 font-medium' : ''}>LDAP 目录</button>
              </div>
            )}

            {error && (
              <div className="text-xs text-red-400 bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2">{error}</div>
            )}

            <button
              type="submit"
              disabled={loading}
              className="w-full py-2.5 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white font-medium transition-all disabled:opacity-60"
            >
              {loading ? '登录中…' : '登录'}
            </button>
          </form>

          {mfaToken && (
            <form onSubmit={verifyMfa} className="space-y-4 mt-4 pt-4 border-t border-slate-800">
              <div className="text-sm text-slate-300">请输入多因素认证（MFA）动态码完成登录</div>
              {!mfaRecovery && (
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">动态验证码 (TOTP)</label>
                  <input
                    type="text"
                    inputMode="numeric"
                    value={mfaCode}
                    onChange={(e) => setMfaCode(e.target.value)}
                    placeholder="123456"
                    className="w-full px-3 py-2.5 rounded-lg bg-slate-800/60 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500 tracking-widest"
                    autoFocus
                  />
                </div>
              )}
              {mfaRecovery && (
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">恢复码</label>
                  <input
                    type="text"
                    value={mfaRecovery}
                    onChange={(e) => setMfaRecovery(e.target.value)}
                    placeholder="xxxx-xxxx-xxxx"
                    className="w-full px-3 py-2.5 rounded-lg bg-slate-800/60 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
                    autoFocus
                  />
                </div>
              )}
              <button
                type="button"
                onClick={() => { setMfaRecovery(''); setMfaCode('') }}
                className="text-xs text-slate-400 hover:text-blue-400"
              >
                {mfaRecovery ? '改用动态验证码' : '使用恢复码'}
              </button>
              {error && (
                <div className="text-xs text-red-400 bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2">{error}</div>
              )}
              <button
                type="submit"
                disabled={loading}
                className="w-full py-2.5 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white font-medium transition-all disabled:opacity-60"
              >
                {loading ? '验证中…' : '验证并登录'}
              </button>
              {isWebAuthnSupported() && (
                <button
                  type="button"
                  onClick={verifyPasskeyLogin}
                  disabled={loading}
                  className="w-full py-2.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-white font-medium transition-all disabled:opacity-60 border border-slate-700"
                >
                  使用 Passkey 登录（指纹 / 面容 / 硬件密钥）
                </button>
              )}
            </form>
          )}
        </div>

        <p className="text-center text-xs text-slate-600 mt-6">Fuze AI PaaS · 推理优先的企业级 AI 底座</p>
      </div>
    </div>
  )
}
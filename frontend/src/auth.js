
const TOKEN_KEY = 'fuze_token'
const USER_KEY = 'fuze_user'

export const API_BASE = (import.meta.env && import.meta.env.VITE_API_BASE_URL) || '/api/v1'

export const POLL_INTERVAL_MS = Number(
  (import.meta.env && import.meta.env.VITE_POLL_INTERVAL_MS) || 5000
)

export function apiUrl(path) {
  if (!path) return API_BASE
  if (/^https?:\/\
    return path
  }
  return API_BASE.replace(/\/$/, '') + '/' + String(path).replace(/^\
}

export const getToken = () => localStorage.getItem(TOKEN_KEY)

export const setToken = (t) => localStorage.setItem(TOKEN_KEY, t)

export const clearToken = () => {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}

export const getUser = () => {
  try {
    return JSON.parse(localStorage.getItem(USER_KEY) || 'null')
  } catch {
    return null
  }
}

export const setUser = (u) => localStorage.setItem(USER_KEY, JSON.stringify(u))

export async function fetchSession(cookieOnly = false) {
  const headers = {}
  if (!cookieOnly) {
    const t = getToken()
    if (t) headers['Authorization'] = 'Bearer ' + t
  }
  try {
    const res = await fetch(apiUrl('/auth/me'), { headers, credentials: 'include' })
    if (!res.ok) return null
    const me = await res.json()
    if (!me || !me.username) return null
    return me
  } catch {
    return null
  }
}

let sessionCache = null 
const SESSION_TTL_MS = 60 * 1000 

export async function getSession() {
  if (sessionCache && Date.now() - sessionCache.at < SESSION_TTL_MS) {
    return sessionCache.me
  }
  const me = await fetchSession(false)
  sessionCache = { me, at: Date.now() }
  return me
}

export function invalidateSessionCache() {
  sessionCache = null
}

export function cookiesSupported() {
  try {
    
    document.cookie = 'fuze_cookie_test=1; path=/; max-age=5'
    const ok = document.cookie.indexOf('fuze_cookie_test=1') !== -1
    document.cookie = 'fuze_cookie_test=; path=/; max-age=0'
    return ok
  } catch {
    return false
  }
}

export async function logout() {
  try {
    await fetch(apiUrl('/auth/logout'), { method: 'POST', credentials: 'include' })
  } catch {
    
  }
  clearToken()
  invalidateSessionCache()
}

export async function apiFetch(path, options = {}) {
  const headers = { ...(options.headers || {}) }
  const token = getToken()
  if (token) {
    headers['Authorization'] = 'Bearer ' + token
  }
  const res = await fetch(apiUrl(path), { ...options, headers, credentials: 'include' })
  if (res.status === 401) {
    handleSessionExpired()
  }
  return res
}

let sessionExpiredRedirecting = false
export function handleSessionExpired() {
  clearToken()
  invalidateSessionCache()
  if (sessionExpiredRedirecting) return
  if (typeof window !== 'undefined' && !window.location.pathname.startsWith('/login')) {
    sessionExpiredRedirecting = true
    window.location.replace('/login?session=expired')
    setTimeout(() => { sessionExpiredRedirecting = false }, 500)
  }
}

export async function apiJson(name, { method = 'GET', body } = {}) {
  const headers = { 'Content-Type': 'application/json' }
  const opts = { method, headers }
  if (body !== undefined) opts.body = JSON.stringify(body)
  const res = await apiFetch(name, opts)
  const text = await res.text()
  
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { raw: text }
    }
  }
  if (!res.ok) {
    const msg =
      (data && (data.error || data.message)) || (data && data.raw) || `请求失败 (${res.status})`
    const err = new Error(msg)
    err.status = res.status
    err.data = data
    
    showApiError(msg)
    throw err
  }
  return data
}

let apiErrorTimer = null
export function showApiError(message) {
  if (typeof document === 'undefined') return
  let el = document.getElementById('fuze-api-error-toast')
  if (!el) {
    el = document.createElement('div')
    el.id = 'fuze-api-error-toast'
    el.style.cssText =
      'position:fixed;top:16px;left:50%;transform:translateX(-50%);z-index:9999;' +
      'background:#7f1d1d;color:#fff;padding:10px 16px;border-radius:8px;' +
      'font-size:13px;max-width:80vw;box-shadow:0 4px 12px rgba(0,0,0,.3);'
    document.body.appendChild(el)
  }
  el.textContent = message
  el.style.display = 'block'
  if (apiErrorTimer) clearTimeout(apiErrorTimer)
  apiErrorTimer = setTimeout(() => {
    el.style.display = 'none'
  }, 3000)
}

export function base64Encode(str) {
  return btoa(unescape(encodeURIComponent(str)))
}

export function isPlatformAdmin() {
  const u = getUser()
  return !!u && u.role === 'platform_admin'
}
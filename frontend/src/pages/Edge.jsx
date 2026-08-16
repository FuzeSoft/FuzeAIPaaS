import React, { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../auth'
import { Plus, Rocket, Trash2, RefreshCw, ShieldAlert, GitBranch, Server } from 'lucide-react'

const TABS = [
  { key: 'nodes', label: '边缘节点' },
  { key: 'deployments', label: '部署与灰度' },
  { key: 'drift', label: '漂移检测' },
]

const statusStyle = (s) => {
  const map = {
    online: 'bg-green-500/20 text-green-400',
    pending: 'bg-blue-500/20 text-blue-400',
    offline: 'bg-red-500/20 text-red-400',
    degraded: 'bg-yellow-500/20 text-yellow-400',
    decommissioning: 'bg-slate-500/20 text-slate-400',
    active: 'bg-green-500/20 text-green-400',
    deploying: 'bg-blue-500/20 text-blue-400',
    rolled_back: 'bg-purple-500/20 text-purple-400',
    failing: 'bg-red-500/20 text-red-400',
  }
  return map[s] || 'bg-slate-500/20 text-slate-400'
}

const severityStyle = (s) => {
  const map = {
    critical: 'bg-red-500/30 text-red-300',
    high: 'bg-orange-500/30 text-orange-300',
    medium: 'bg-yellow-500/30 text-yellow-300',
    low: 'bg-blue-500/30 text-blue-300',
    none: 'bg-slate-500/20 text-slate-400',
  }
  return map[s] || 'bg-slate-500/20 text-slate-400'
}

export function edgeSubmitLabelFeedback(depId, payload) {
  return apiFetch(`/api/v1/edge-deployments/${depId}/label-feedback`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

const Edge = () => {
  const [tab, setTab] = useState('nodes')
  const [nodes, setNodes] = useState([])
  const [deployments, setDeployments] = useState([])
  const [drift, setDrift] = useState({})
  const [loading, setLoading] = useState(true)
  const [showNodeModal, setShowNodeModal] = useState(false)
  const [showDeployModal, setShowDeployModal] = useState(false)
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState(null)
  const [labelFeedback, setLabelFeedback] = useState({})
  const [newNode, setNewNode] = useState({ name: '', mode: 'agent', endpoint: '', region: '' })
  const [newDeploy, setNewDeploy] = useState({
    nodeId: '', modelId: '', version: '', image: '', replicas: 1, canaryWeight: 0,
    autoRollback: true, driftGuard: true,
  })

  const flash = (m, ok = true) => {
    setMsg({ text: m, ok })
    setTimeout(() => setMsg(null), 3000)
  }

  const fetchNodes = useCallback(async () => {
    try {
      const res = await apiFetch('/api/v1/edge-nodes')
      const data = await res.json()
      setNodes(Array.isArray(data?.nodes) ? data.nodes : [])
    } catch (e) { console.error(e) }
  }, [])

  const fetchDeployments = useCallback(async () => {
    try {
      const res = await apiFetch('/api/v1/edge-deployments')
      const data = await res.json()
      setDeployments(Array.isArray(data?.deployments) ? data.deployments : [])
    } catch (e) { console.error(e) }
  }, [])

  const fetchDrift = useCallback(async () => {
    const out = {}
    await Promise.all(deployments.map(async (d) => {
      try {
        const res = await apiFetch(`/api/v1/edge-deployments/${d.id}/drift`)
        if (res.ok) out[d.id] = await res.json()
      } catch (e) {  }
    }))
    setDrift(out)
  }, [deployments])

  useEffect(() => {
    setLoading(true)
    Promise.all([fetchNodes(), fetchDeployments()]).finally(() => setLoading(false))
    const t = setInterval(() => { fetchNodes(); fetchDeployments() }, 8000)
    return () => clearInterval(t)
  }, [fetchNodes, fetchDeployments])

  useEffect(() => { if (tab === 'drift') fetchDrift() }, [tab, fetchDrift])

  const handleRegisterNode = async (e) => {
    e.preventDefault()
    setBusy(true)
    try {
      await apiFetch('/api/v1/edge-nodes', { method: 'POST', body: JSON.stringify(newNode) })
      setShowNodeModal(false)
      setNewNode({ name: '', mode: 'agent', endpoint: '', region: '' })
      fetchNodes()
      flash('节点已纳管')
    } catch (err) { flash('纳管失败: ' + err.message, false) }
    finally { setBusy(false) }
  }

  const handleDeploy = async (e) => {
    e.preventDefault()
    setBusy(true)
    try {
      await apiFetch('/api/v1/edge-deployments', { method: 'POST', body: JSON.stringify(newDeploy) })
      setShowDeployModal(false)
      setNewDeploy({ nodeId: '', modelId: '', version: '', image: '', replicas: 1, canaryWeight: 0, autoRollback: true, driftGuard: true })
      fetchDeployments()
      flash('已下发部署')
    } catch (err) { flash('下发失败: ' + err.message, false) }
    finally { setBusy(false) }
  }

  const promoteCanary = async (id) => {
    setBusy(true)
    try {
      await apiFetch(`/api/v1/edge-deployments/${id}/canary/promote`, { method: 'POST', body: JSON.stringify({ step: 25 }) })
      fetchDeployments()
      flash('灰度已推进')
    } catch (err) { flash('推进失败: ' + err.message, false) }
    finally { setBusy(false) }
  }

  const rollback = async (id) => {
    if (!confirm('确认回滚到稳定版本？')) return
    setBusy(true)
    try {
      await apiFetch(`/api/v1/edge-deployments/${id}/rollback`, { method: 'POST', body: JSON.stringify({ reason: 'manual' }) })
      fetchDeployments()
      flash('已回滚')
    } catch (err) { flash('回滚失败: ' + err.message, false) }
    finally { setBusy(false) }
  }

  const runDrift = async (id) => {
    setBusy(true)
    try {
      const res = await apiFetch(`/api/v1/edge-deployments/${id}/drift/check`, { method: 'POST' })
      const data = await res.json()
      setDrift((prev) => ({ ...prev, [id]: data }))
      flash(data.triggeredRollback ? '漂移触发，已自动回滚' : '漂移检测完成')
    } catch (err) { flash('检测失败: ' + err.message, false) }
    finally { setBusy(false) }
  }

  const submitLabelFeedback = async (id, e) => {
    e.preventDefault()
    const form = labelFeedback[id] || { label: '', requestId: '' }
    if (!form.label.trim()) { flash('请填写回标标签', false); return }
    setBusy(true)
    try {
      const res = await edgeSubmitLabelFeedback(id, { label: form.label.trim(), requestId: form.requestId.trim() })
      if (res.ok || res.status === 204) {
        flash('回标已提交，概念漂移将纳入真实标签')
        setLabelFeedback((prev) => ({ ...prev, [id]: { label: '', requestId: '' } }))
      } else {
        const err = await res.json().catch(() => ({}))
        flash('回标失败: ' + (err.error || res.status), false)
      }
    } catch (err) { flash('回标失败: ' + err.message, false) }
    finally { setBusy(false) }
  }

  const deregisterNode = async (id) => {
    if (!confirm('确认注销该节点？')) return
    try {
      await apiFetch(`/api/v1/edge-nodes/${id}`, { method: 'DELETE' })
      fetchNodes()
      flash('节点已注销')
    } catch (err) { flash('注销失败: ' + err.message, false) }
  }

  return (
    <div className="p-8">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white flex items-center gap-2">
            <Server className="w-6 h-6 text-blue-400" /> 边缘部署与漂移防护
          </h1>
          <p className="text-slate-400 text-sm mt-1">边缘节点纳管 · 灰度下发 · 数据/预测/性能/概念漂移检测与自动回滚</p>
        </div>
        <div className="flex gap-2">
          {tab === 'nodes' && (
            <button onClick={() => setShowNodeModal(true)} className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-500 rounded-lg text-white text-sm font-medium">
              <Plus className="w-4 h-4" /> 纳管节点
            </button>
          )}
          {tab === 'deployments' && (
            <button onClick={() => setShowDeployModal(true)} className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-500 rounded-lg text-white text-sm font-medium">
              <Rocket className="w-4 h-4" /> 下发部署
            </button>
          )}
          <button onClick={() => { fetchNodes(); fetchDeployments(); if (tab === 'drift') fetchDrift() }} className="flex items-center gap-2 px-4 py-2 bg-slate-700 hover:bg-slate-600 rounded-lg text-white text-sm">
            <RefreshCw className="w-4 h-4" /> 刷新
          </button>
        </div>
      </div>

      {msg && (
        <div className={`mb-4 px-4 py-2 rounded-lg text-sm ${msg.ok ? 'bg-green-500/20 text-green-300' : 'bg-red-500/20 text-red-300'}`}>{msg.text}</div>
      )}

      <div className="flex gap-2 mb-6 border-b border-slate-800">
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${tab === t.key ? 'border-blue-500 text-white' : 'border-transparent text-slate-400 hover:text-white'}`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {loading && <p className="text-slate-400">加载中…</p>}

      {tab === 'nodes' && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {nodes.map((n) => (
            <div key={n.id} className="bg-slate-900/60 border border-slate-800 rounded-xl p-4">
              <div className="flex items-center justify-between mb-2">
                <h3 className="text-white font-semibold">{n.name}</h3>
                <span className={`px-2 py-1 rounded text-xs ${statusStyle(n.status)}`}>{n.status}</span>
              </div>
              <p className="text-slate-400 text-xs">模式: {n.mode} · 区域: {n.region || '-'}</p>
              <p className="text-slate-400 text-xs truncate">{n.endpoint}</p>
              <div className="flex gap-2 mt-3">
                <button onClick={() => apiFetch(`/api/v1/edge-nodes/${n.id}/heartbeat`, { method: 'POST' }).then(fetchNodes)} className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 rounded text-white">心跳</button>
                <button onClick={() => deregisterNode(n.id)} className="text-xs px-3 py-1 bg-red-600/70 hover:bg-red-500 rounded text-white flex items-center gap-1"><Trash2 className="w-3 h-3" /> 注销</button>
              </div>
            </div>
          ))}
          {!loading && nodes.length === 0 && <p className="text-slate-500">暂无边缘节点，点击「纳管节点」开始。</p>}
        </div>
      )}

      {tab === 'deployments' && (
        <div className="space-y-3">
          {deployments.map((d) => (
            <div key={d.id} className="bg-slate-900/60 border border-slate-800 rounded-xl p-4 flex items-center justify-between">
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="text-white font-semibold">{d.modelId}@{d.version}</h3>
                  <span className={`px-2 py-1 rounded text-xs ${statusStyle(d.status)}`}>{d.status}</span>
                  {d.driftGuardEnabled && <span className="px-2 py-1 rounded text-xs bg-purple-500/20 text-purple-300">漂移护栏</span>}
                </div>
                <p className="text-slate-400 text-xs mt-1">节点 {d.nodeId} · 灰度 {d.canaryWeight}% · 自动回滚 {d.autoRollback ? '开' : '关'}</p>
              </div>
              <div className="flex gap-2">
                {d.canaryWeight > 0 && d.canaryWeight < 100 && (
                  <button onClick={() => promoteCanary(d.id)} disabled={busy} className="text-xs px-3 py-1 bg-blue-600 hover:bg-blue-500 rounded text-white flex items-center gap-1"><GitBranch className="w-3 h-3" /> 推进灰度</button>
                )}
                <button onClick={() => runDrift(d.id)} disabled={busy} className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 rounded text-white flex items-center gap-1"><ShieldAlert className="w-3 h-3" /> 漂移检测</button>
                <button onClick={() => rollback(d.id)} disabled={busy} className="text-xs px-3 py-1 bg-red-600/70 hover:bg-red-500 rounded text-white">回滚</button>
              </div>
            </div>
          ))}
          {!loading && deployments.length === 0 && <p className="text-slate-500">暂无部署，点击「下发部署」开始。</p>}
        </div>
      )}

      {tab === 'drift' && (
        <div className="space-y-3">
          {deployments.map((d) => {
            const r = drift[d.id]
            const fb = labelFeedback[d.id] || { label: '', requestId: '' }
            const concept = r?.conceptDrift
            const conceptEnabled = concept?.drifted !== undefined
            const conceptHasLabel = concept?.detail === 'label distribution TVD'
            const conceptSkipped = concept?.detail?.includes('no ground truth')
            return (
              <div key={d.id} className="bg-slate-900/60 border border-slate-800 rounded-xl p-4">
                <div className="flex items-center justify-between mb-2">
                  <h3 className="text-white font-semibold">{d.modelId}@{d.version}</h3>
                  <button onClick={() => runDrift(d.id)} disabled={busy} className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 rounded text-white">重新检测</button>
                </div>
                {!r && <p className="text-slate-500 text-sm">尚无漂移报告，点击「重新检测」。</p>}

                {r && (
                  <div className="space-y-3 mt-2">
                    {}
                    <div className="space-y-2">
                      {[['数据', r.dataDrift], ['预测', r.predictionDrift], ['性能', r.performanceDrift], ['概念', r.conceptDrift]].map(([label, m]) => {
                        const score = m?.score ?? 0
                        const threshold = m?.threshold ?? 0
                        const pct = threshold > 0 ? Math.min(100, (score / threshold) * 100) : 0
                        const over = !!m?.drifted
                        return (
                          <div key={label} className="flex items-center gap-3">
                            <span className="w-12 text-slate-400 text-xs">{label}</span>
                            <div className="flex-1 h-2 bg-slate-800 rounded-full overflow-hidden relative">
                              {}
                              <div className="absolute top-0 bottom-0 w-px bg-slate-500" style={{ left: '100%' }} />
                              <div
                                className={`h-full rounded-full ${over ? 'bg-red-500' : 'bg-blue-500'}`}
                                style={{ width: `${pct}%` }}
                              />
                            </div>
                            <span className={`w-28 text-right text-xs ${over ? 'text-red-400' : 'text-slate-400'}`}>
                              {m ? `${score.toFixed(3)} / ${threshold.toFixed(3)}` : 'n/a'}
                            </span>
                            {over && <span className="px-2 py-0.5 rounded text-xs bg-red-500/30 text-red-300">超阈值</span>}
                          </div>
                        )
                      })}
                    </div>

                    {}
                    <div className="bg-slate-800/40 rounded-lg p-3 border border-slate-700/60">
                      <div className="flex items-center justify-between mb-2">
                        <p className="text-white text-sm font-medium">概念漂移 · 标签回标</p>
                        {conceptEnabled ? (
                          conceptSkipped ? (
                            <span className="px-2 py-1 rounded text-xs bg-yellow-500/20 text-yellow-300">待回标真实标签</span>
                          ) : (
                            <span className={`px-2 py-1 rounded text-xs ${conceptHasLabel ? 'bg-green-500/20 text-green-300' : 'bg-yellow-500/20 text-yellow-300'}`}>
                              {conceptHasLabel ? '已纳入真实标签' : '已计算'}
                            </span>
                          )
                        ) : (
                          <span className="px-2 py-1 rounded text-xs bg-slate-500/20 text-slate-400">未配置</span>
                        )}
                      </div>
                      <p className="text-slate-400 text-xs mb-2">
                        回标推理网关的真实标签，检测器据此计算标签分布漂移（TVD）。不填 requestId 则记为批量回标。
                      </p>
                      <form onSubmit={(e) => submitLabelFeedback(d.id, e)} className="flex flex-wrap items-end gap-2">
                        <label className="block">
                          <span className="text-slate-400 text-xs">回标标签</span>
                          <input
                            className="bg-slate-700 border border-slate-600 rounded-lg px-3 py-1.5 text-white text-sm focus:outline-none focus:border-blue-500"
                            placeholder="如 positive / cat"
                            value={fb.label}
                            onChange={(e) => setLabelFeedback((prev) => ({ ...prev, [d.id]: { ...fb, label: e.target.value } }))}
                          />
                        </label>
                        <label className="block">
                          <span className="text-slate-400 text-xs">Request ID（可选）</span>
                          <input
                            className="bg-slate-700 border border-slate-600 rounded-lg px-3 py-1.5 text-white text-sm focus:outline-none focus:border-blue-500"
                            placeholder="推理请求追踪 ID"
                            value={fb.requestId}
                            onChange={(e) => setLabelFeedback((prev) => ({ ...prev, [d.id]: { ...fb, requestId: e.target.value } }))}
                          />
                        </label>
                        <button disabled={busy} className="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 rounded text-white text-xs font-medium">提交回标</button>
                      </form>
                    </div>

                    <div className="flex items-center gap-3">
                      <span className={`px-2 py-1 rounded text-xs ${severityStyle(r.overallSeverity)}`}>总体: {r.overallSeverity}</span>
                      {r.triggeredRollback && <span className="px-2 py-1 rounded text-xs bg-purple-500/20 text-purple-300">已触发自动回滚</span>}
                      <span className="text-slate-400 text-xs">{r.recommendation}</span>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
          {!loading && deployments.length === 0 && <p className="text-slate-500">暂无部署，无法检测漂移。</p>}
        </div>
      )}

      {showNodeModal && (
        <Modal title="纳管边缘节点" onClose={() => setShowNodeModal(false)}>
          <form onSubmit={handleRegisterNode} className="space-y-3">
            <Field label="名称"><input className="inp" value={newNode.name} onChange={(e) => setNewNode({ ...newNode, name: e.target.value })} required /></Field>
            <Field label="模式">
              <select className="inp" value={newNode.mode} onChange={(e) => setNewNode({ ...newNode, mode: e.target.value })}>
                <option value="agent">Agent (HTTPS+mTLS)</option>
                <option value="kubeedge">KubeEdge</option>
              </select>
            </Field>
            <Field label="接入端点"><input className="inp" placeholder="https://edge-1:8443" value={newNode.endpoint} onChange={(e) => setNewNode({ ...newNode, endpoint: e.target.value })} required /></Field>
            <Field label="区域"><input className="inp" value={newNode.region} onChange={(e) => setNewNode({ ...newNode, region: e.target.value })} /></Field>
            <button disabled={busy} className="w-full py-2 bg-blue-600 hover:bg-blue-500 rounded-lg text-white font-medium">纳管</button>
          </form>
        </Modal>
      )}

      {showDeployModal && (
        <Modal title="下发模型到边缘" onClose={() => setShowDeployModal(false)}>
          <form onSubmit={handleDeploy} className="space-y-3">
            <Field label="节点 ID"><input className="inp" value={newDeploy.nodeId} onChange={(e) => setNewDeploy({ ...newDeploy, nodeId: e.target.value })} required /></Field>
            <Field label="模型 ID"><input className="inp" value={newDeploy.modelId} onChange={(e) => setNewDeploy({ ...newDeploy, modelId: e.target.value })} required /></Field>
            <Field label="版本"><input className="inp" value={newDeploy.version} onChange={(e) => setNewDeploy({ ...newDeploy, version: e.target.value })} required /></Field>
            <Field label="镜像"><input className="inp" placeholder="registry/model-serve:latest" value={newDeploy.image} onChange={(e) => setNewDeploy({ ...newDeploy, image: e.target.value })} required /></Field>
            <Field label="副本数"><input type="number" className="inp" value={newDeploy.replicas} onChange={(e) => setNewDeploy({ ...newDeploy, replicas: +e.target.value })} /></Field>
            <Field label="灰度权重%"><input type="number" className="inp" value={newDeploy.canaryWeight} onChange={(e) => setNewDeploy({ ...newDeploy, canaryWeight: +e.target.value })} /></Field>
            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={newDeploy.autoRollback} onChange={(e) => setNewDeploy({ ...newDeploy, autoRollback: e.target.checked })} /> 自动回滚</label>
            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={newDeploy.driftGuard} onChange={(e) => setNewDeploy({ ...newDeploy, driftGuard: e.target.checked })} /> 漂移护栏</label>
            <button disabled={busy} className="w-full py-2 bg-blue-600 hover:bg-blue-500 rounded-lg text-white font-medium">下发</button>
          </form>
        </Modal>
      )}
    </div>
  )
}

const Modal = ({ title, onClose, children }) => (
  <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
    <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-96" onClick={(e) => e.stopPropagation()}>
      <h2 className="text-white font-semibold mb-4">{title}</h2>
      {children}
    </div>
  </div>
)

const Field = ({ label, children }) => (
  <label className="block">
    <span className="text-slate-400 text-xs">{label}</span>
    <div className="mt-1">{children}</div>
  </label>
)

export default Edge
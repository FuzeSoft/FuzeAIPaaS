import React, { useState, useEffect } from 'react'
import { apiFetch } from '../auth'
import { Plus, Rocket, Boxes, Trash2, Tag, User, GitBranch, Package, Layers, Network } from 'lucide-react'

const Models = () => {
  const [models, setModels] = useState([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState({})
  const [versionsByModel, setVersionsByModel] = useState({})
  const [showModelModal, setShowModelModal] = useState(false)
  const [showVersionModal, setShowVersionModal] = useState(false)
  const [activeModelId, setActiveModelId] = useState(null)
  const [newModel, setNewModel] = useState({ name: '', framework: 'pytorch', owner: '', description: '' })
  const [newVersion, setNewVersion] = useState({ version: '', storageUri: '', image: '', sizeBytes: 0 })
  
  const [lineage, setLineage] = useState(null)
  const [lineageLoading, setLineageLoading] = useState(false)
  const [lineageErr, setLineageErr] = useState('')
  const [lineageTarget, setLineageTarget] = useState(null) 

  useEffect(() => { fetchModels() }, [])

  const fetchModels = async () => {
    try {
      const res = await apiFetch('/api/v1/models')
      const data = await res.json()
      setModels(Array.isArray(data) ? data : [])
    } catch (e) {
      console.error('Error fetching models:', e)
    } finally {
      setLoading(false)
    }
  }

  const fetchVersions = async (modelId) => {
    try {
      const res = await apiFetch(`/api/v1/models/${modelId}/versions`)
      const data = await res.json()
      setVersionsByModel((prev) => ({ ...prev, [modelId]: Array.isArray(data) ? data : [] }))
    } catch (e) {
      console.error('Error fetching versions:', e)
    }
  }

  const toggleExpand = (modelId) => {
    const next = !expanded[modelId]
    setExpanded((prev) => ({ ...prev, [modelId]: next }))
    if (next && !versionsByModel[modelId]) fetchVersions(modelId)
  }

  const handleCreateModel = async (e) => {
    e.preventDefault()
    try {
      const res = await apiFetch('/api/v1/models', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newModel),
      })
      if (res.ok) {
        setShowModelModal(false)
        setNewModel({ name: '', framework: 'pytorch', owner: '', description: '' })
        fetchModels()
      }
    } catch (e) {
      console.error('Error creating model:', e)
    }
  }

  const handleCreateVersion = async (e) => {
    e.preventDefault()
    try {
      const res = await apiFetch(`/api/v1/models/${activeModelId}/versions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newVersion),
      })
      if (res.ok) {
        setShowVersionModal(false)
        setNewVersion({ version: '', storageUri: '', image: '', sizeBytes: 0 })
        fetchVersions(activeModelId)
      }
    } catch (e) {
      console.error('Error creating version:', e)
    }
  }

  const handleDeleteModel = async (id) => {
    if (!confirm('确定删除该模型及其全部版本？')) return
    try {
      await apiFetch(`/api/v1/models/${id}`, { method: 'DELETE' })
      fetchModels()
    } catch (e) {
      console.error('Error deleting model:', e)
    }
  }

  const handleDeleteVersion = async (modelId, vid) => {
    if (!confirm('确定删除该模型版本？')) return
    try {
      await apiFetch(`/api/v1/models/${modelId}/versions/${vid}`, { method: 'DELETE' })
      fetchVersions(modelId)
    } catch (e) {
      console.error('Error deleting version:', e)
    }
  }

  const handleDeriveInference = async (m, v) => {
    const payload = {
      name: `${m.name}-${v.version}`,
      framework: m.framework || 'pytorch',
      runtime: 'kserve',
      storage_uri: v.storageUri || v.image || '',
      image: v.image || '',
      min_replicas: 1,
      max_replicas: 1,
      cpu: '2',
      memory: '8Gi',
      gpus: 1,
      gpu_memory: 0,
      gpu_cores: 0,
      cluster_id: 'cluster-001',
      chip: 'nvidia',
    }
    try {
      
      const res = await apiFetch('/api/v1/inference-services', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ spec: payload }),
      })
      if (res.ok) {
        alert(`已基于 ${m.name} / ${v.version} 派生推理服务：${payload.name}`)
      } else {
        alert('派生推理服务失败')
      }
    } catch (e) {
      console.error('Error deriving inference service:', e)
      alert('派生推理服务失败：' + e.message)
    }
  }

  const openLineage = async (m, v) => {
    setLineageTarget({ modelId: m.id, vid: v.id, label: `${m.name} / ${v.version}` })
    setLineage(null)
    setLineageErr('')
    setLineageLoading(true)
    try {
      const res = await apiFetch(`/api/v1/models/${m.id}/versions/${v.id}/lineage`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setLineage(data)
    } catch (e) {
      console.error('Error fetching lineage:', e)
      setLineageErr('血缘信息获取失败：' + e.message)
    } finally {
      setLineageLoading(false)
    }
  }

  const closeLineage = () => {
    setLineageTarget(null)
    setLineage(null)
  }

  if (loading) {
    return <div className="flex items-center justify-center min-h-screen"><div className="text-slate-400">加载中...</div></div>
  }

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2">模型仓库</h1>
            <p className="text-slate-400">推理优先：统一管理模型与版本，一键派生推理服务</p>
          </div>
          <button
            onClick={() => setShowModelModal(true)}
            className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl"
          >
            <Plus className="w-5 h-5" />
            注册模型
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {models.map((m) => {
            const versions = versionsByModel[m.id] || []
            const isOpen = expanded[m.id]
            return (
              <div key={m.id} className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6 hover:border-blue-500/50 transition-all">
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-xl flex items-center justify-center">
                      <Boxes className="w-5 h-5 text-white" />
                    </div>
                    <div>
                      <h3 className="font-bold text-white">{m.name}</h3>
                      <span className="text-xs text-slate-400 uppercase">{m.framework}</span>
                    </div>
                  </div>
                  <button onClick={() => handleDeleteModel(m.id)} className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg transition-colors">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>

                <p className="text-sm text-slate-400 mb-3 line-clamp-2">{m.description || '暂无描述'}</p>

                <div className="space-y-2 text-sm">
                  <div className="flex justify-between"><span className="text-slate-400 flex items-center gap-1"><User className="w-3.5 h-3.5" />负责人</span><span className="text-slate-300">{m.owner || '—'}</span></div>
                  <div className="flex justify-between"><span className="text-slate-400 flex items-center gap-1"><GitBranch className="w-3.5 h-3.5" />版本数</span><span className="text-slate-300">{versions.length || (isOpen ? 0 : '—')}</span></div>
                </div>

                <button
                  onClick={() => toggleExpand(m.id)}
                  className="mt-4 w-full flex items-center justify-center gap-2 text-sm text-blue-400 hover:text-blue-300 py-2 rounded-lg border border-slate-700 hover:border-blue-500/50 transition-colors"
                >
                  <Layers className="w-4 h-4" />
                  {isOpen ? '收起版本' : '查看版本'}
                </button>

                {isOpen && (
                  <div className="mt-3 space-y-2">
                    {versions.map((v) => (
                      <div key={v.id} className="flex items-center justify-between bg-slate-900/60 rounded-lg px-3 py-2">
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <Tag className="w-3.5 h-3.5 text-cyan-400" />
                            <span className="text-sm text-white font-medium">{v.version}</span>
                          </div>
                          <div className="text-[11px] text-slate-400 truncate max-w-[230px]" title={v.storageUri || v.image}>
                            {v.storageUri || v.image || '-'}
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          {v.sizeBytes ? <span className="text-[11px] text-slate-400 flex items-center gap-1"><Package className="w-3 h-3" />{v.sizeBytes} bytes</span> : null}
                          <button onClick={() => handleDeriveInference(m, v)} title="一键派生推理服务" className="p-1.5 text-slate-400 hover:text-green-400 hover:bg-slate-700 rounded transition-colors">
                            <Rocket className="w-3.5 h-3.5" />
                          </button>
                          <button onClick={() => openLineage(m, v)} title="查看完整血缘" className="p-1.5 text-slate-400 hover:text-cyan-400 hover:bg-slate-700 rounded transition-colors">
                            <Network className="w-3.5 h-3.5" />
                          </button>
                          <button onClick={() => handleDeleteVersion(m.id, v.id)} className="p-1.5 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded transition-colors">
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </div>
                    ))}
                    {versions.length === 0 && <div className="text-center text-xs text-slate-500 py-3">暂无版本</div>}
                    <button
                      onClick={() => { setActiveModelId(m.id); setShowVersionModal(true) }}
                      className="w-full flex items-center justify-center gap-2 text-sm text-green-400 hover:text-green-300 py-2 rounded-lg border border-dashed border-slate-600 hover:border-green-500/50 transition-colors"
                    >
                      <Plus className="w-4 h-4" />
                      新增版本
                    </button>
                  </div>
                )}
              </div>
            )
          })}
          {models.length === 0 && (
            <div className="col-span-full text-center text-slate-500 py-16">暂无模型，点击「注册模型」创建</div>
          )}
        </div>

        {}
        {showModelModal && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-lg border border-slate-700 shadow-2xl">
              <h2 className="text-2xl font-bold text-white mb-6">注册模型</h2>
              <form onSubmit={handleCreateModel} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">模型名称</label>
                  <input type="text" value={newModel.name} onChange={(e) => setNewModel({ ...newModel, name: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" required />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">框架</label>
                    <select value={newModel.framework} onChange={(e) => setNewModel({ ...newModel, framework: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                      <option value="pytorch">PyTorch</option>
                      <option value="tensorflow">TensorFlow</option>
                      <option value="triton">Triton</option>
                      <option value="sklearn">SKLearn</option>
                      <option value="onnx">ONNX</option>
                      <option value="custom">自定义</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">负责人</label>
                    <input type="text" value={newModel.owner} onChange={(e) => setNewModel({ ...newModel, owner: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="团队/个人" />
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">描述</label>
                  <textarea value={newModel.description} onChange={(e) => setNewModel({ ...newModel, description: e.target.value })} rows={3} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="模型用途、训练数据等" />
                </div>
                <div className="flex gap-3 mt-6">
                  <button type="button" onClick={() => setShowModelModal(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">取消</button>
                  <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all">注册</button>
                </div>
              </form>
            </div>
          </div>
        )}

        {}
        {showVersionModal && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-lg border border-slate-700 shadow-2xl">
              <h2 className="text-2xl font-bold text-white mb-6">新增模型版本</h2>
              <form onSubmit={handleCreateVersion} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">版本号</label>
                  <input type="text" value={newVersion.version} onChange={(e) => setNewVersion({ ...newVersion, version: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="v1.0 / 20260101" required />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">模型存储地址 (storageUri)</label>
                  <input type="text" value={newVersion.storageUri} onChange={(e) => setNewVersion({ ...newVersion, storageUri: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="s3://models/... 或 pvc://model-store/..." />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">镜像 (可选)</label>
                    <input type="text" value={newVersion.image} onChange={(e) => setNewVersion({ ...newVersion, image: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="myregistry/model:tag" />
                  </div>
                  <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">大小 (字节)</label>
                  <input type="number" value={newVersion.sizeBytes} onChange={(e) => setNewVersion({ ...newVersion, sizeBytes: parseInt(e.target.value) || 0 })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="如 2450000000" min="0" />
                  </div>
                </div>
                <div className="flex gap-3 mt-6">
                  <button type="button" onClick={() => setShowVersionModal(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">取消</button>
                  <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all">新增</button>
                </div>
              </form>
            </div>
          </div>
        )}

        {}
        {lineageTarget && (
          <LineageModal
            target={lineageTarget}
            graph={lineage}
            loading={lineageLoading}
            error={lineageErr}
            onClose={closeLineage}
          />
        )}
      </div>
    </div>
  )
}

const NODE_META = {
  code: { color: '#22d3ee', label: '代码', icon: '⌥' },
  data: { color: '#a78bfa', label: '数据', icon: '▤' },
  hyperparam: { color: '#f472b6', label: '超参', icon: 'λ' },
  job: { color: '#60a5fa', label: '训练任务', icon: '⚙' },
  run: { color: '#34d399', label: '实验运行', icon: '⚡' },
  'model-version': { color: '#fbbf24', label: '模型版本', icon: '◈' },
}

const LineageModal = ({ target, graph, loading, error, onClose }) => {
  
  const layerOf = (type) => {
    if (type === 'model-version') return 4
    if (type === 'job' || type === 'run') return 3
    return 1 
  }
  const nodeById = {}
  if (graph) graph.nodes.forEach((n) => (nodeById[n.id] = n))

  const layers = {}
  if (graph) {
    graph.nodes.forEach((n) => {
      const l = layerOf(n.type)
      ;(layers[l] = layers[l] || []).push(n)
    })
  }
  const COL_W = 240
  const ROW_H = 90
  const coords = {}
  Object.keys(layers).map(Number).sort((a, b) => a - b).forEach((l) => {
    layers[l].forEach((n, i) => {
      coords[n.id] = { x: l * COL_W + 40, y: i * ROW_H + 40 }
    })
  })

  const width = 5 * COL_W
  const height = (graph ? Object.values(coords).length : 1) * ROW_H + 40

  return (
    <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4" onClick={onClose}>
      <div className="bg-slate-800 rounded-2xl p-6 w-full max-w-5xl border border-slate-700 shadow-2xl" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-xl font-bold text-white flex items-center gap-2">
              <Network className="w-5 h-5 text-cyan-400" /> 模型血缘
            </h2>
            <p className="text-sm text-slate-400 mt-1">{target.label} · code → data → hyperparam → 训练 → 版本</p>
          </div>
          <button onClick={onClose} className="p-2 text-slate-400 hover:text-white hover:bg-slate-700 rounded-lg transition-colors">✕</button>
        </div>

        {loading && <div className="text-slate-400 py-12 text-center">加载血缘…</div>}
        {error && <div className="text-red-400 py-12 text-center">{error}</div>}

        {!loading && !error && graph && graph.nodes.length === 0 && (
          <div className="text-slate-500 py-12 text-center">暂无血缘信息</div>
        )}

        {!loading && !error && graph && graph.nodes.length > 0 && (
          <div className="overflow-auto bg-slate-900/60 rounded-xl border border-slate-700 p-2">
            <svg width={width} height={height} className="min-w-full">
              {}
              {graph.edges.map((e, i) => {
                const a = coords[e.from], b = coords[e.to]
                if (!a || !b) return null
                const midX = (a.x + b.x) / 2
                return (
                  <path
                    key={i}
                    d={`M ${a.x + 80} ${a.y + 28} C ${midX} ${a.y + 28}, ${midX} ${b.y + 28}, ${b.x} ${b.y + 28}`}
                    stroke="#475569"
                    strokeWidth="2"
                    fill="none"
                    markerEnd="url(#arrow)"
                  />
                )
              })}
              <defs>
                <marker id="arrow" markerWidth="10" markerHeight="10" refX="8" refY="3" orient="auto" markerUnits="strokeWidth">
                  <path d="M0,0 L8,3 L0,6 Z" fill="#64748b" />
                </marker>
              </defs>
              {}
              {graph.nodes.map((n) => {
                const c = coords[n.id]
                const meta = NODE_META[n.type] || { color: '#94a3b8', label: n.type, icon: '•' }
                return (
                  <g key={n.id} transform={`translate(${c.x},${c.y})`}>
                    <rect width="160" height="56" rx="12" fill="#0f172a" stroke={meta.color} strokeWidth="1.5" />
                    <circle cx="20" cy="28" r="12" fill={meta.color + '22'} stroke={meta.color} />
                    <text x="20" y="33" textAnchor="middle" fill={meta.color} fontSize="12">{meta.icon}</text>
                    <text x="40" y="22" fill="#e2e8f0" fontSize="12" fontWeight="600">{meta.label}</text>
                    <text x="40" y="40" fill="#94a3b8" fontSize="10">
                      {n.label ? (n.label.length > 18 ? n.label.slice(0, 18) + '…' : n.label) : n.id}
                    </text>
                  </g>
                )
              })}
            </svg>
          </div>
        )}

        {}
        {!loading && !error && graph && graph.nodes.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 mt-4 max-h-64 overflow-auto">
            {graph.nodes.map((n) => {
              const meta = NODE_META[n.type] || { color: '#94a3b8', label: n.type }
              const attrs = n.attributes || {}
              return (
                <div key={n.id} className="bg-slate-900/60 rounded-xl border border-slate-700 p-3">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="w-2.5 h-2.5 rounded-full" style={{ background: meta.color }} />
                    <span className="text-sm font-medium text-white">{meta.label}</span>
                    {n.label && <span className="text-[11px] text-slate-400 truncate ml-auto max-w-[120px]">{n.label}</span>}
                  </div>
                  {Object.keys(attrs).length === 0 ? (
                    <div className="text-[11px] text-slate-500">无附加属性</div>
                  ) : (
                    <div className="space-y-1">
                      {Object.entries(attrs).map(([k, v]) => (
                        <div key={k} className="text-[11px] flex gap-2">
                          <span className="text-slate-500 w-16 shrink-0">{k}</span>
                          <span className="text-slate-300 break-all">{typeof v === 'string' && v.length > 60 ? v.slice(0, 60) + '…' : String(v)}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

export default Models
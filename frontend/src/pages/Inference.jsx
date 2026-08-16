import React, { useState, useEffect } from 'react'
import { apiFetch } from '../auth'
import { Plus, Rocket, Trash2, ExternalLink, Cpu, Scale, GitBranch, RefreshCw } from 'lucide-react'
import { inferenceStatusStyle } from '../utils/status'

const Inference = () => {
  const [services, setServices] = useState([])
  const [clusters, setClusters] = useState([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [newSvc, setNewSvc] = useState({
    name: '',
    framework: 'pytorch',
    runtime: 'kserve',
    storage_uri: '',
    image: '',
    min_replicas: 1,
    max_replicas: 3,
    cpu: '2',
    memory: '8Gi',
    gpus: 1,
    gpu_memory: 0,
    gpu_cores: 0,
    cluster_id: 'cluster-001',
    chip: 'nvidia',
  })

  const [showScale, setShowScale] = useState(false)
  const [scaleSvc, setScaleSvc] = useState(null)
  const [scaleVal, setScaleVal] = useState(1)
  const [showCanary, setShowCanary] = useState(false)
  const [canarySvc, setCanarySvc] = useState(null)
  const [canaryVal, setCanaryVal] = useState(0)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    fetchServices()
    fetchClusters()
    const timer = setInterval(fetchServices, 5000)
    return () => clearInterval(timer)
  }, [])

  const fetchServices = async () => {
    try {
      const res = await apiFetch('/api/v1/inference-services')
      const data = await res.json()
      setServices(Array.isArray(data) ? data : [])
    } catch (e) {
      console.error('Error fetching inference services:', e)
    } finally {
      setLoading(false)
    }
  }

  const fetchClusters = async () => {
    try {
      const res = await apiFetch('/api/v1/clusters')
      setClusters(await res.json())
    } catch (e) {
      console.error('Error fetching clusters:', e)
    }
  }

  const handleCreate = async (e) => {
    e.preventDefault()
    try {
      
      const res = await apiFetch('/api/v1/inference-services', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ spec: newSvc }),
      })
      if (res.ok) {
        setShowModal(false)
        fetchServices()
      }
    } catch (e) {
      console.error('Error creating inference service:', e)
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除该推理服务？')) return
    try {
      await apiFetch(`/api/v1/inference-services/${id}`, { method: 'DELETE' })
      fetchServices()
    } catch (e) {
      console.error('Error deleting inference service:', e)
    }
  }

  const patchSpec = async (id, spec) => {
    setBusy(true)
    try {
      const res = await apiFetch(`/api/v1/inference-services/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ spec }),
      })
      if (res.ok) fetchServices()
      return res.ok
    } catch (e) {
      console.error('Error patching inference service:', e)
      return false
    } finally {
      setBusy(false)
    }
  }

  const openScale = (svc) => {
    setScaleSvc(svc)
    setScaleVal(svc.spec.target_replicas || svc.spec.min_replicas || 1)
    setShowScale(true)
  }
  const handleScale = async (e) => {
    e.preventDefault()
    if (await patchSpec(scaleSvc.id, { target_replicas: Number(scaleVal) })) setShowScale(false)
  }

  const openCanary = (svc) => {
    setCanarySvc(svc)
    setCanaryVal(svc.spec.canary_weight || 0)
    setShowCanary(true)
  }
  const handleCanary = async (e) => {
    e.preventDefault()
    if (await patchSpec(canarySvc.id, { canary_weight: Number(canaryVal) })) setShowCanary(false)
  }

  const clusterNameOf = (id) => clusters.find((c) => c.id === id)?.name || id || '—'

  if (loading) {
    return <div className="flex items-center justify-center min-h-screen"><div className="text-slate-400">加载中...</div></div>
  }

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2">推理服务</h1>
            <p className="text-slate-400">推理优先：多运行时部署、弹性扩缩与灰度发布（支持 Scale-to-Zero）</p>
          </div>
          <button
            onClick={() => setShowModal(true)}
            className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl"
          >
            <Plus className="w-5 h-5" />
            部署服务
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {services.map((svc) => {
            const spec = svc.spec || {}
            const status = svc.status || {}
            const st = inferenceStatusStyle(status.phase)
            const StatusIcon = st.icon
            return (
              <div key={svc.id} className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6 hover:border-blue-500/50 transition-all">
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-xl flex items-center justify-center">
                      <Rocket className="w-5 h-5 text-white" />
                    </div>
                    <div>
                      <h3 className="font-bold text-white">{spec.name}</h3>
                      <span className="text-xs text-slate-400 uppercase">{spec.framework}</span>
                      {spec.runtime ? <span className="ml-2 text-[10px] text-cyan-400 bg-cyan-500/10 px-1.5 py-0.5 rounded">{spec.runtime}</span> : null}
                      {spec.chip ? <span className="ml-1 text-[10px] text-purple-300 bg-purple-500/10 px-1.5 py-0.5 rounded">{spec.chip}</span> : null}
                      <div className="text-[10px] text-slate-400 mt-0.5">{clusterNameOf(spec.cluster_id)}</div>
                    </div>
                  </div>
                  <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium ${st.bg} ${st.text}`}>
                    <StatusIcon className="w-3.5 h-3.5" />
                    {st.label}
                  </span>
                </div>

                <div className="space-y-2 text-sm">
                  <div className="flex justify-between"><span className="text-slate-400">模型地址</span><span className="text-slate-300 truncate max-w-[200px]" title={spec.storage_uri}>{spec.storage_uri || spec.image || '-'}</span></div>
                  <div className="flex justify-between"><span className="text-slate-400">副本范围</span><span className="text-slate-300">{spec.min_replicas} ~ {spec.max_replicas}（就绪 {status.ready_replicas || 0}{spec.target_replicas ? ` / 目标 ${spec.target_replicas}` : ''}）</span></div>
                  <div className="flex justify-between"><span className="text-slate-400">资源</span><span className="text-slate-300">{spec.cpu} 核 / {spec.memory} / {spec.gpus} GPU</span></div>
                  {spec.canary_weight > 0 && (
                    <div className="flex justify-between"><span className="text-slate-400 flex items-center gap-1"><GitBranch className="w-3.5 h-3.5" /> 灰度流量</span><span className="text-purple-400 text-xs">{spec.canary_weight}%</span></div>
                  )}
                  {(spec.gpu_memory > 0 || spec.gpu_cores > 0) && (
                    <div className="flex justify-between items-center">
                      <span className="text-slate-400 flex items-center gap-1"><Cpu className="w-3.5 h-3.5" /> HAMi 隔离</span>
                      <span className="text-cyan-400 text-xs">{spec.gpu_memory > 0 ? `${spec.gpu_memory}MiB` : ''} {spec.gpu_cores > 0 ? `${spec.gpu_cores}%算力` : ''}</span>
                    </div>
                  )}
                </div>

                <div className="flex items-center justify-between mt-4 pt-4 border-t border-slate-700">
                  {status.url ? (
                    <a href={status.url} target="_blank" rel="noreferrer" className="flex items-center gap-1 text-blue-400 hover:text-blue-300 text-xs truncate max-w-[160px]">
                      <ExternalLink className="w-3.5 h-3.5" /> {status.url}
                    </a>
                  ) : <span className="text-slate-500 text-xs">等待分配地址...</span>}
                  <div className="flex items-center gap-1">
                    <button onClick={fetchServices} title="刷新观测态（收敛由控制循环负责）" className="p-2 text-slate-400 hover:text-blue-400 hover:bg-slate-700 rounded-lg transition-colors">
                      <RefreshCw className="w-4 h-4" />
                    </button>
                    <button onClick={() => openCanary(svc)} title="灰度发布" className="p-2 text-slate-400 hover:text-purple-400 hover:bg-slate-700 rounded-lg transition-colors">
                      <GitBranch className="w-4 h-4" />
                    </button>
                    <button onClick={() => openScale(svc)} title="扩缩容" className="p-2 text-slate-400 hover:text-green-400 hover:bg-slate-700 rounded-lg transition-colors">
                      <Scale className="w-4 h-4" />
                    </button>
                    <button onClick={() => handleDelete(svc.id)} title="删除" className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg transition-colors">
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              </div>
            )
          })}
          {services.length === 0 && (
            <div className="col-span-full text-center text-slate-500 py-16">暂无推理服务，点击「部署服务」创建</div>
          )}
        </div>
      </div>

      {showModal && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-lg border border-slate-700 shadow-2xl max-h-[90vh] overflow-y-auto">
            <h2 className="text-2xl font-bold text-white mb-6">部署推理服务</h2>
            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">服务名称</label>
                <input type="text" value={newSvc.name} onChange={(e) => setNewSvc({ ...newSvc, name: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" required />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">预测器框架</label>
                <select value={newSvc.framework} onChange={(e) => setNewSvc({ ...newSvc, framework: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                  <option value="pytorch">PyTorch</option>
                  <option value="tensorflow">TensorFlow</option>
                  <option value="triton">Triton</option>
                  <option value="sklearn">SKLearn</option>
                  <option value="xgboost">XGBoost</option>
                  <option value="onnx">ONNX</option>
                  <option value="custom">自定义镜像</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">推理运行时</label>
                <select value={newSvc.runtime} onChange={(e) => setNewSvc({ ...newSvc, runtime: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                  <option value="kserve">KServe（兼容）</option>
                  <option value="vllm">vLLM（大模型推理）</option>
                  <option value="triton">Triton（多框架）</option>
                  <option value="ascend">Ascend（昇腾·信创）</option>
                  <option value="custom">自定义</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">目标集群</label>
                <select value={newSvc.cluster_id} onChange={(e) => setNewSvc({ ...newSvc, cluster_id: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                  {clusters.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">目标芯片厂商（信创底座）</label>
                <select value={newSvc.chip} onChange={(e) => setNewSvc({ ...newSvc, chip: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                  <option value="nvidia">NVIDIA</option>
                  <option value="ascend">华为昇腾 (Ascend)</option>
                  <option value="cambricon">寒武纪 (Cambricon)</option>
                </select>
              </div>
              {newSvc.framework === 'custom' ? (
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">容器镜像</label>
                  <input type="text" value={newSvc.image} onChange={(e) => setNewSvc({ ...newSvc, image: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="myregistry/my-predictor:latest" />
                </div>
              ) : (
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">模型存储地址 (storageUri)</label>
                  <input type="text" value={newSvc.storage_uri} onChange={(e) => setNewSvc({ ...newSvc, storage_uri: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="s3://models/... 或 pvc://model-store/..." />
                </div>
              )}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">最小副本 (0=缩容到零)</label>
                  <input type="number" value={newSvc.min_replicas} onChange={(e) => setNewSvc({ ...newSvc, min_replicas: parseInt(e.target.value) })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" min="0" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">最大副本</label>
                  <input type="number" value={newSvc.max_replicas} onChange={(e) => setNewSvc({ ...newSvc, max_replicas: parseInt(e.target.value) })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" min="1" />
                </div>
              </div>
              <div className="grid grid-cols-3 gap-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">CPU</label>
                  <input type="text" value={newSvc.cpu} onChange={(e) => setNewSvc({ ...newSvc, cpu: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">内存</label>
                  <input type="text" value={newSvc.memory} onChange={(e) => setNewSvc({ ...newSvc, memory: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">GPU</label>
                  <input type="number" value={newSvc.gpus} onChange={(e) => setNewSvc({ ...newSvc, gpus: parseInt(e.target.value) })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" min="0" />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4 p-4 bg-slate-900/50 rounded-xl border border-slate-700">
                <div className="col-span-2 text-xs text-cyan-400 flex items-center gap-1"><Cpu className="w-3.5 h-3.5" /> HAMi GPU 显存/算力隔离（0 表示整卡独占）</div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">显存 (MiB)</label>
                  <input type="number" value={newSvc.gpu_memory} onChange={(e) => setNewSvc({ ...newSvc, gpu_memory: parseInt(e.target.value) })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" min="0" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">算力 (%)</label>
                  <input type="number" value={newSvc.gpu_cores} onChange={(e) => setNewSvc({ ...newSvc, gpu_cores: parseInt(e.target.value) })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" min="0" max="100" />
                </div>
              </div>
              <div className="flex gap-3 mt-6">
                <button type="button" onClick={() => setShowModal(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">取消</button>
                <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all">部署</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {}
      {showScale && scaleSvc && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-md border border-slate-700 shadow-2xl">
            <h2 className="text-2xl font-bold text-white mb-2">弹性扩缩容</h2>
            <p className="text-sm text-slate-400 mb-6">服务：<span className="text-blue-400">{scaleSvc.spec.name}</span></p>
            <form onSubmit={handleScale} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">期望副本数（范围 {scaleSvc.spec.min_replicas} ~ {scaleSvc.spec.max_replicas}）</label>
                <input type="number" value={scaleVal} onChange={(e) => setScaleVal(parseInt(e.target.value))} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" min={scaleSvc.spec.min_replicas} max={scaleSvc.spec.max_replicas} />
              </div>
              <div className="flex gap-3 mt-6">
                <button type="button" onClick={() => setShowScale(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">取消</button>
                <button type="submit" disabled={busy} className="flex-1 bg-gradient-to-r from-green-600 to-teal-600 hover:from-green-700 hover:to-teal-700 text-white px-6 py-3 rounded-xl font-medium transition-all disabled:opacity-50">{busy ? '下发中...' : '确认扩缩'}</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {}
      {showCanary && canarySvc && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-md border border-slate-700 shadow-2xl">
            <h2 className="text-2xl font-bold text-white mb-2">灰度发布（金丝雀）</h2>
            <p className="text-sm text-slate-400 mb-6">服务：<span className="text-purple-400">{canarySvc.spec.name}</span></p>
            <form onSubmit={handleCanary} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">新版本流量权重：<span className="text-purple-400 font-bold">{canaryVal}%</span></label>
                <input type="range" min="0" max="100" value={canaryVal} onChange={(e) => setCanaryVal(parseInt(e.target.value))} className="w-full accent-purple-500" />
                <div className="flex justify-between text-xs text-slate-500 mt-1"><span>0%（仅稳定版）</span><span>100%（全量新版本）</span></div>
              </div>
              <div className="flex gap-3 mt-6">
                <button type="button" onClick={() => setShowCanary(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">取消</button>
                <button type="submit" disabled={busy} className="flex-1 bg-gradient-to-r from-purple-600 to-pink-600 hover:from-purple-700 hover:to-pink-700 text-white px-6 py-3 rounded-xl font-medium transition-all disabled:opacity-50">{busy ? '下发中...' : '确认灰度'}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}

export default Inference
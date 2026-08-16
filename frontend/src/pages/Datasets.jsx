import React, { useState, useEffect } from 'react'
import { apiFetch } from '../auth'
import { Plus, Database, Trash2, HardDrive, Zap } from 'lucide-react'
import { datasetStatusStyle } from '../utils/status'

const Datasets = () => {
  const [datasets, setDatasets] = useState([])
  const [clusters, setClusters] = useState([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [newDs, setNewDs] = useState({
    name: '',
    mount_point: '',
    runtime: 'alluxio',
    replicas: 2,
    cache_capacity: '100Gi',
    cache_medium: 'SSD',
    access_mode: 'ReadOnly',
    cluster_id: 'cluster-001',
  })

  useEffect(() => {
    fetchDatasets()
    fetchClusters()
    const timer = setInterval(fetchDatasets, 5000)
    return () => clearInterval(timer)
  }, [])

  const fetchDatasets = async () => {
    try {
      const res = await apiFetch('/api/v1/datasets')
      const data = await res.json()
      setDatasets(Array.isArray(data) ? data : [])
    } catch (e) {
      console.error('Error fetching datasets:', e)
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
      const res = await apiFetch('/api/v1/datasets', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newDs),
      })
      if (res.ok) {
        setShowModal(false)
        fetchDatasets()
      }
    } catch (e) {
      console.error('Error creating dataset:', e)
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除该数据集？')) return
    try {
      await apiFetch(`/api/v1/datasets/${id}`, { method: 'DELETE' })
      fetchDatasets()
    } catch (e) {
      console.error('Error deleting dataset:', e)
    }
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
            <h1 className="text-3xl font-bold text-white mb-2">数据集</h1>
            <p className="text-slate-400">基于 Fluid 的数据编排与缓存加速，提升训练 IO 吞吐</p>
          </div>
          <button
            onClick={() => setShowModal(true)}
            className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl"
          >
            <Plus className="w-5 h-5" />
            新建数据集
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {datasets.map((ds) => {
            const st = datasetStatusStyle(ds.status)
            const StatusIcon = st.icon
            return (
              <div key={ds.id} className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6 hover:border-blue-500/50 transition-all">
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 bg-gradient-to-br from-cyan-500 to-blue-600 rounded-xl flex items-center justify-center">
                      <Database className="w-5 h-5 text-white" />
                    </div>
                    <div>
                      <h3 className="font-bold text-white">{ds.name}</h3>
                      <span className="text-xs text-slate-400 uppercase">{ds.runtime}</span>
                      <div className="text-[10px] text-slate-400 mt-0.5">{clusterNameOf(ds.cluster_id)}</div>
                    </div>
                  </div>
                  <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium ${st.bg} ${st.text}`}>
                    <StatusIcon className="w-3.5 h-3.5" />
                    {st.label}
                  </span>
                </div>

                <div className="space-y-2 text-sm">
                  <div className="flex justify-between"><span className="text-slate-400">底层存储</span><span className="text-slate-300 truncate max-w-[200px]" title={ds.mount_point}>{ds.mount_point}</span></div>
                  <div className="flex justify-between"><span className="text-slate-400 flex items-center gap-1"><HardDrive className="w-3.5 h-3.5" />缓存</span><span className="text-slate-300">{ds.cache_capacity} × {ds.replicas} ({ds.cache_medium})</span></div>
                  <div className="flex justify-between"><span className="text-slate-400">数据量</span><span className="text-slate-300">{ds.ufs_total || '-'}</span></div>
                </div>

                <div className="mt-4">
                  <div className="flex justify-between text-xs mb-1">
                    <span className="text-slate-400 flex items-center gap-1"><Zap className="w-3.5 h-3.5 text-yellow-400" />缓存命中率</span>
                    <span className="text-slate-300">{(ds.cached_percent || 0).toFixed(1)}%</span>
                  </div>
                  <div className="w-full bg-slate-700 rounded-full h-2">
                    <div className="bg-gradient-to-r from-cyan-500 to-blue-500 h-2 rounded-full transition-all" style={{ width: `${Math.min(ds.cached_percent || 0, 100)}%` }}></div>
                  </div>
                </div>

                <div className="flex items-center justify-end mt-4 pt-4 border-t border-slate-700">
                  <button onClick={() => handleDelete(ds.id)} className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg transition-colors">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            )
          })}
          {datasets.length === 0 && (
            <div className="col-span-full text-center text-slate-500 py-16">暂无数据集，点击「新建数据集」创建</div>
          )}
        </div>
      </div>

      {showModal && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-lg border border-slate-700 shadow-2xl max-h-[90vh] overflow-y-auto">
            <h2 className="text-2xl font-bold text-white mb-6">新建数据集</h2>
            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">数据集名称</label>
                <input type="text" value={newDs.name} onChange={(e) => setNewDs({ ...newDs, name: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" required />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">底层存储地址 (mountPoint)</label>
                <input type="text" value={newDs.mount_point} onChange={(e) => setNewDs({ ...newDs, mount_point: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="oss://bucket/path 或 s3://... 或 pvc://..." required />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">缓存运行时</label>
                  <select value={newDs.runtime} onChange={(e) => setNewDs({ ...newDs, runtime: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                    <option value="alluxio">Alluxio</option>
                    <option value="juicefs">JuiceFS</option>
                    <option value="goosefs">GooseFS</option>
                    <option value="vineyard">Vineyard</option>
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">缓存介质</label>
                  <select value={newDs.cache_medium} onChange={(e) => setNewDs({ ...newDs, cache_medium: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                    <option value="MEM">内存 (MEM)</option>
                    <option value="SSD">SSD</option>
                    <option value="HDD">HDD</option>
                  </select>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">缓存副本数</label>
                  <input type="number" value={newDs.replicas} onChange={(e) => setNewDs({ ...newDs, replicas: parseInt(e.target.value) })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" min="1" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">单副本缓存容量</label>
                  <input type="text" value={newDs.cache_capacity} onChange={(e) => setNewDs({ ...newDs, cache_capacity: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500" placeholder="100Gi" />
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">访问模式</label>
                <select value={newDs.access_mode} onChange={(e) => setNewDs({ ...newDs, access_mode: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                  <option value="ReadOnly">只读 (ReadOnly)</option>
                  <option value="ReadWrite">读写 (ReadWrite)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">目标集群</label>
                <select value={newDs.cluster_id} onChange={(e) => setNewDs({ ...newDs, cluster_id: e.target.value })} className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500">
                  {clusters.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
              </div>
              <div className="flex gap-3 mt-6">
                <button type="button" onClick={() => setShowModal(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors">取消</button>
                <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all">创建</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}

export default Datasets
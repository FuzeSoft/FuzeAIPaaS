import React, { useState, useEffect } from 'react'
import { apiFetch } from '../auth'
import { Cpu, Search, Plus, Activity, HardDrive } from 'lucide-react'
import { resourceStatusStyle } from '../utils/status'

const Resources = () => {
  const [resources, setResources] = useState([])
  const [clusters, setClusters] = useState([])
  const [selectedCluster, setSelectedCluster] = useState('')
  const [searchTerm, setSearchTerm] = useState('')
  const [loading, setLoading] = useState(true)
  const [showResModal, setShowResModal] = useState(false)
  const [newRes, setNewRes] = useState({ name: '', type: 'GPU', vendor: '', model: '', total_gpus: 8, total_memory: 80, cluster_id: '', node_name: '' })

  useEffect(() => {
    fetchClusters()
    fetchResources('')
  }, [])

  const fetchClusters = async () => {
    try {
      const res = await apiFetch('/api/v1/clusters')
      setClusters(await res.json())
    } catch (error) {
      console.error('Error fetching clusters:', error)
    }
  }

  const fetchResources = async (clusterId) => {
    try {
      const url = clusterId ? `/api/v1/resources?cluster_id=${clusterId}` : '/api/v1/resources'
      const response = await apiFetch(url)
      const data = await response.json()
      setResources(data)
    } catch (error) {
      console.error('Error fetching resources:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleClusterChange = (e) => {
    const clusterId = e.target.value
    setSelectedCluster(clusterId)
    fetchResources(clusterId)
  }

  const handleCreateResource = async (e) => {
    e.preventDefault()
    try {
      const res = await apiFetch('/api/v1/resources', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...newRes, status: 'available' }),
      })
      if (res.ok) {
        setShowResModal(false)
        setNewRes({ name: '', type: 'GPU', vendor: '', model: '', total_gpus: 8, total_memory: 80, cluster_id: '', node_name: '' })
        fetchResources(selectedCluster)
      }
    } catch (error) {
      console.error('Error creating resource:', error)
    }
  }

  const clusterNameOf = (id) => clusters.find((c) => c.id === id)?.name || id || '—'

  const filteredResources = resources.filter(resource =>
    (resource.name || '').toLowerCase().includes(searchTerm.toLowerCase()) ||
    (resource.model || '').toLowerCase().includes(searchTerm.toLowerCase()) ||
    clusterNameOf(resource.cluster_id).toLowerCase().includes(searchTerm.toLowerCase())
  )

  const getTypeIcon = (type) => {
    switch (type) {
      case 'GPU': return <Cpu className="w-5 h-5" />
      case 'NPU': return <Activity className="w-5 h-5" />
      default: return <Cpu className="w-5 h-5" />
    }
  }

  const getTypeColor = (type) => {
    switch (type) {
      case 'GPU': return 'text-blue-400'
      case 'NPU': return 'text-purple-400'
      default: return 'text-cyan-400'
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-slate-400">加载中...</div>
      </div>
    )
  }

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2">资源管理</h1>
            <p className="text-slate-400">管理集群中的计算资源</p>
          </div>
          <button onClick={() => setShowResModal(true)} className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl">
            <Plus className="w-5 h-5" />
            添加资源
          </button>
        </div>

        <div className="mb-6 flex flex-col md:flex-row gap-4">
          <select
            value={selectedCluster}
            onChange={handleClusterChange}
            className="bg-slate-800 border border-slate-700 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all"
          >
            <option value="">全部集群</option>
            {clusters.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
          <div className="relative flex-1">
            <Search className="absolute left-4 top-1/2 transform -translate-y-1/2 text-slate-400 w-5 h-5" />
            <input
              type="text"
              placeholder="搜索资源..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded-xl pl-12 pr-4 py-3 text-white placeholder-slate-400 focus:outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all"
            />
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {filteredResources.map((resource) => {
            const statusStyle = resourceStatusStyle(resource.status)
            const totalMem = Number(resource.total_memory) || 0
            const usedMem = Math.max(0, totalMem - (Number(resource.available_memory) || 0))
            const memoryUsage = totalMem > 0 ? Math.min(100, (usedMem / totalMem) * 100) : 0
            return (
              <div key={resource.id} className="bg-gradient-to-br from-slate-800 to-slate-900 rounded-2xl p-6 border border-slate-700 shadow-xl hover:shadow-2xl transition-all duration-300 hover:-translate-y-1">
                <div className="flex items-start justify-between mb-4">
                  <div className={`w-14 h-14 bg-slate-700/50 rounded-xl flex items-center justify-center ${getTypeColor(resource.type)}`}>
                    {getTypeIcon(resource.type)}
                  </div>
                  <span className={`px-3 py-1 rounded-full text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.label}
                  </span>
                </div>
                <h3 className="text-lg font-semibold text-white mb-1">{resource.name}</h3>
                <p className="text-slate-400 text-sm mb-4">{(resource.vendor || '-')} {(resource.model || '-')} · {clusterNameOf(resource.cluster_id)}</p>
                <div className="space-y-3">
                  <div>
                    <div className="flex justify-between text-sm mb-1">
                      <span className="text-slate-400">显存</span>
                      <span className="text-white">{usedMem} / {totalMem} GiB</span>
                    </div>
                    <div className="w-full bg-slate-700 rounded-full h-2">
                      <div
                        className="bg-gradient-to-r from-blue-500 to-purple-500 h-2 rounded-full transition-all duration-500"
                        style={{ width: `${memoryUsage}%` }}
                      ></div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 text-sm text-slate-400">
                    <HardDrive className="w-4 h-4" />
                    <span>节点: {resource.node_name || '-'}</span>
                  </div>
                </div>
              </div>
            )
          })}
        </div>

        {showResModal && (
          <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-md border border-slate-700">
              <h2 className="text-xl font-bold text-white mb-6">添加资源</h2>
              <form onSubmit={handleCreateResource} className="space-y-4">
                <input required placeholder="资源名称" value={newRes.name} onChange={(e) => setNewRes({ ...newRes, name: e.target.value })} className="w-full bg-slate-900 border border-slate-700 rounded-xl px-4 py-3 text-white" />
                <select value={newRes.type} onChange={(e) => setNewRes({ ...newRes, type: e.target.value })} className="w-full bg-slate-900 border border-slate-700 rounded-xl px-4 py-3 text-white">
                  <option value="GPU">GPU</option>
                  <option value="NPU">NPU</option>
                  <option value="CPU">CPU</option>
                </select>
                <input placeholder="厂商" value={newRes.vendor} onChange={(e) => setNewRes({ ...newRes, vendor: e.target.value })} className="w-full bg-slate-900 border border-slate-700 rounded-xl px-4 py-3 text-white" />
                <input placeholder="型号" value={newRes.model} onChange={(e) => setNewRes({ ...newRes, model: e.target.value })} className="w-full bg-slate-900 border border-slate-700 rounded-xl px-4 py-3 text-white" />
                <div className="grid grid-cols-2 gap-4">
                  <input type="number" min="0" placeholder="总卡数" value={newRes.total_gpus} onChange={(e) => setNewRes({ ...newRes, total_gpus: Number(e.target.value) })} className="w-full bg-slate-900 border border-slate-700 rounded-xl px-4 py-3 text-white" />
                  <input type="number" min="0" placeholder="单卡显存(GiB)" value={newRes.total_memory} onChange={(e) => setNewRes({ ...newRes, total_memory: Number(e.target.value) })} className="w-full bg-slate-900 border border-slate-700 rounded-xl px-4 py-3 text-white" />
                </div>
                <select value={newRes.cluster_id} onChange={(e) => setNewRes({ ...newRes, cluster_id: e.target.value })} className="w-full bg-slate-900 border border-slate-700 rounded-xl px-4 py-3 text-white">
                  <option value="">选择集群</option>
                  {clusters.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
                <input placeholder="节点名称" value={newRes.node_name} onChange={(e) => setNewRes({ ...newRes, node_name: e.target.value })} className="w-full bg-slate-900 border border-slate-700 rounded-xl px-4 py-3 text-white" />
                <div className="flex gap-3 pt-2">
                  <button type="button" onClick={() => setShowResModal(false)} className="flex-1 bg-slate-700 hover:bg-slate-600 text-white py-3 rounded-xl">取消</button>
                  <button type="submit" className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white py-3 rounded-xl">创建</button>
                </div>
              </form>
            </div>
          </div>
        )}

      </div>
    </div>
  )
}

export default Resources
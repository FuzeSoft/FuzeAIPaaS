import React, { useState, useEffect } from 'react'
import { apiFetch } from '../auth'
import { Plus, MonitorSmartphone, Trash2, Play, Square, ExternalLink, Clock, Cpu, HardDrive } from 'lucide-react'
import { workspaceStatusStyle } from '../utils/status'

const Workspaces = () => {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [newWs, setNewWs] = useState({
    name: '',
    kind: 'notebook',
    owner_id: 'user-1',
    image: 'registry.example.com/fuze-notebook:latest',
    gpu_count: 1,
    gpu_model: 'nvidia-a100',
    cpu_request: '4',
    memory_request: '16Gi',
    idle_timeout_seconds: 3600,
  })

  useEffect(() => {
    fetchWorkspaces()
    const timer = setInterval(fetchWorkspaces, 5000)
    return () => clearInterval(timer)
  }, [])

  const fetchWorkspaces = async () => {
    try {
      const res = await apiFetch('/api/v1/workspaces')
      const data = await res.json()
      setItems(data.items || [])
    } catch (e) {
      console.error('Error fetching workspaces:', e)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (e) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      const res = await apiFetch('/api/v1/workspaces', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newWs),
      })
      if (res.ok) {
        setShowModal(false)
        setNewWs({ ...newWs, name: '', image: 'registry.example.com/fuze-notebook:latest' })
        fetchWorkspaces()
      }
    } catch (e) {
      console.error('Error creating workspace:', e)
    } finally {
      setSubmitting(false)
    }
  }

  const handleAction = async (id, action) => {
    try {
      await apiFetch(`/api/v1/workspaces/${id}/${action}`, { method: 'POST' })
      fetchWorkspaces()
    } catch (e) {
      console.error(`Error ${action} workspace:`, e)
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除该工作空间？容器与数据卷将被回收。')) return
    try {
      await apiFetch(`/api/v1/workspaces/${id}`, { method: 'DELETE' })
      fetchWorkspaces()
    } catch (e) {
      console.error('Error deleting workspace:', e)
    }
  }

  const openProxy = (id) => {
    
    window.open(`/api/v1/workspaces/${id}/proxy/`, '_blank')
  }

  if (loading) {
    return <div className="flex items-center justify-center min-h-screen"><div className="text-slate-400">加载中...</div></div>
  }

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2">Notebook 工作空间</h1>
            <p className="text-slate-400">交互式开发环境：按需拉起 GPU 容器，闲置自动回收</p>
          </div>
          <button
            onClick={() => setShowModal(true)}
            className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl"
          >
            <Plus className="w-5 h-5" />
            新建工作空间
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {items.map((ws) => {
            const st = workspaceStatusStyle(ws.status)
            const StatusIcon = st.icon
            const isRunning = ws.status === 'running'
            const isStopped = ws.status === 'stopped' || ws.status === 'failed'
            return (
              <div key={ws.id} className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6 hover:border-blue-500/50 transition-all">
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 bg-gradient-to-br from-cyan-500 to-blue-600 rounded-xl flex items-center justify-center">
                      <MonitorSmartphone className="w-5 h-5 text-white" />
                    </div>
                    <div>
                      <h3 className="font-bold text-white">{ws.name}</h3>
                      <span className="text-xs text-slate-400 uppercase">{ws.kind || 'notebook'}</span>
                      <div className="text-[10px] text-slate-400 mt-0.5">owner: {ws.owner_id}</div>
                    </div>
                  </div>
                  <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium ${st.bg} ${st.text}`}>
                    <StatusIcon className="w-3.5 h-3.5" />
                    {st.label}
                  </span>
                </div>

                <div className="space-y-2 text-sm">
                  <div className="flex justify-between"><span className="text-slate-400 flex items-center gap-1"><Cpu className="w-3.5 h-3.5" />GPU</span><span className="text-slate-300">{ws.gpu_count || 0} × {ws.gpu_model || '—'}</span></div>
                  <div className="flex justify-between"><span className="text-slate-400 flex items-center gap-1"><Cpu className="w-3.5 h-3.5" />CPU</span><span className="text-slate-300">{ws.cpu_request || '—'}</span></div>
                  <div className="flex justify-between"><span className="text-slate-400 flex items-center gap-1"><HardDrive className="w-3.5 h-3.5" />内存</span><span className="text-slate-300">{ws.memory_request || '—'}</span></div>
                  <div className="flex justify-between"><span className="text-slate-400 flex items-center gap-1"><Clock className="w-3.5 h-3.5" />空闲回收</span><span className="text-slate-300">{ws.idle_timeout_seconds ? `${ws.idle_timeout_seconds}s` : '关闭'}</span></div>
                  <div className="text-[11px] text-slate-500 truncate" title={ws.image}>{ws.image}</div>
                </div>

                <div className="flex items-center justify-end gap-1 mt-4 pt-4 border-t border-slate-700">
                  {isRunning && (
                    <button onClick={() => openProxy(ws.id)} title="打开 Web 终端" className="p-2 text-slate-400 hover:text-blue-400 hover:bg-slate-700 rounded-lg transition-colors">
                      <ExternalLink className="w-4 h-4" />
                    </button>
                  )}
                  {isStopped && (
                    <button onClick={() => handleAction(ws.id, 'start')} title="启动" className="p-2 text-slate-400 hover:text-green-400 hover:bg-slate-700 rounded-lg transition-colors">
                      <Play className="w-4 h-4" />
                    </button>
                  )}
                  {!isStopped && (
                    <button onClick={() => handleAction(ws.id, 'stop')} title="停止" className="p-2 text-slate-400 hover:text-yellow-400 hover:bg-slate-700 rounded-lg transition-colors">
                      <Square className="w-4 h-4" />
                    </button>
                  )}
                  <button onClick={() => handleDelete(ws.id)} title="删除" className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg transition-colors">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            )
          })}
          {items.length === 0 && (
            <div className="col-span-full text-center py-16 text-slate-500">
              暂无工作空间，点击右上角「新建工作空间」开始。
            </div>
          )}
        </div>
      </div>

      {showModal && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-800 rounded-2xl border border-slate-700 p-8 w-full max-w-lg shadow-2xl">
            <h2 className="text-xl font-bold text-white mb-6">新建工作空间</h2>
            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-sm text-slate-400 mb-1">名称</label>
                <input
                  type="text" required value={newWs.name}
                  onChange={(e) => setNewWs({ ...newWs, name: e.target.value })}
                  className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white focus:border-blue-500 outline-none"
                  placeholder="my-notebook"
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm text-slate-400 mb-1">GPU 数量</label>
                  <input type="number" min="0" value={newWs.gpu_count}
                    onChange={(e) => setNewWs({ ...newWs, gpu_count: parseInt(e.target.value || '0', 10) })}
                    className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white focus:border-blue-500 outline-none" />
                </div>
                <div>
                  <label className="block text-sm text-slate-400 mb-1">GPU 型号</label>
                  <input type="text" value={newWs.gpu_model}
                    onChange={(e) => setNewWs({ ...newWs, gpu_model: e.target.value })}
                    className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white focus:border-blue-500 outline-none" />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm text-slate-400 mb-1">CPU</label>
                  <input type="text" value={newWs.cpu_request}
                    onChange={(e) => setNewWs({ ...newWs, cpu_request: e.target.value })}
                    className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white focus:border-blue-500 outline-none" placeholder="4" />
                </div>
                <div>
                  <label className="block text-sm text-slate-400 mb-1">内存</label>
                  <input type="text" value={newWs.memory_request}
                    onChange={(e) => setNewWs({ ...newWs, memory_request: e.target.value })}
                    className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white focus:border-blue-500 outline-none" placeholder="16Gi" />
                </div>
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">镜像</label>
                <input type="text" value={newWs.image}
                  onChange={(e) => setNewWs({ ...newWs, image: e.target.value })}
                  className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white focus:border-blue-500 outline-none" />
              </div>
              <div>
                <label className="block text-sm text-slate-400 mb-1">空闲回收超时（秒，0=关闭）</label>
                <input type="number" min="0" value={newWs.idle_timeout_seconds}
                  onChange={(e) => setNewWs({ ...newWs, idle_timeout_seconds: parseInt(e.target.value || '0', 10) })}
                  className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white focus:border-blue-500 outline-none" />
              </div>
              <div className="flex justify-end gap-3 pt-2">
                <button type="button" onClick={() => setShowModal(false)}
                  className="px-5 py-2.5 rounded-lg text-slate-300 bg-slate-700/60 hover:bg-slate-600 transition-colors">取消</button>
                <button type="submit" disabled={submitting}
                  className="px-5 py-2.5 rounded-lg bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white font-medium transition-all disabled:opacity-50">
                  {submitting ? '创建中...' : '创建'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}

export default Workspaces
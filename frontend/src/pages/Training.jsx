import React, { useState, useEffect } from 'react'
import { apiFetch } from '../auth'
import { Plus, Search, Trash2, FileText, Clock, RefreshCw, Ban, RotateCcw, CheckCircle2, Save } from 'lucide-react'
import { jobStatusStyle } from '../utils/status'

const emptyJob = {
  name: '',
  template_id: '',
  image: '',
  command: '',
  gpus: 1,
  memory: 80,
  priority: 100,
  max_runtime: 0,
  gpu_memory: 0,
  gpu_cores: 0,
  distributed: false,
  framework: 'pytorch-ddp',
  replicas: 2,
  min_available: 0,
  dataset_name: '',
  mount_path: '/data',
  code_commit: '',
  cluster_id: 'cluster-001',
  checkpointing: { enabled: false, interval_steps: 500, max_retries: 3 },
  register_model: { enabled: false, model_id: '', version_tag: '' },
}

const formatDuration = (ms) => {
  const totalMin = Math.floor(ms / 60000)
  if (totalMin >= 60) return `${Math.floor(totalMin / 60)} 小时 ${totalMin % 60} 分`
  if (totalMin >= 1) return `${totalMin} 分钟`
  return `${Math.max(0, Math.floor(ms / 1000))} 秒`
}

const runtimeRemaining = (job, nowMs) => {
  if (job.status !== 'running' || !job.max_runtime || !job.started_at) return null
  const startedAt = new Date(job.started_at).getTime()
  if (Number.isNaN(startedAt)) return null
  const leftMs = startedAt + job.max_runtime * 60000 - nowMs
  return { leftMs, overdue: leftMs <= 0, text: formatDuration(Math.abs(leftMs)) }
}

const Training = () => {
  const [jobs, setJobs] = useState([])
  const [datasets, setDatasets] = useState([])
  const [clusters, setClusters] = useState([])
  const [templates, setTemplates] = useState([])
  const [actionError, setActionError] = useState('')
  const [searchTerm, setSearchTerm] = useState('')
  const [filterStatus, setFilterStatus] = useState('all')
  const [showModal, setShowModal] = useState(false)
  const [loading, setLoading] = useState(true)
  const [newJob, setNewJob] = useState({ ...emptyJob })
  
  const [logJob, setLogJob] = useState(null)
  const [logData, setLogData] = useState(null)
  const [logLoading, setLogLoading] = useState(false)
  const [logQuery, setLogQuery] = useState({ pod: '', task: '', tail: 100 })
  
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    fetchJobs()
    fetchDatasets()
    fetchClusters()
    fetchTemplates()
  }, [])

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30000)
    return () => clearInterval(timer)
  }, [])

  const fetchJobs = async () => {
    try {
      const response = await apiFetch('/api/v1/training-jobs')
      const data = await response.json()
      setJobs(data)
    } catch (error) {
      console.error('Error fetching jobs:', error)
    } finally {
      setLoading(false)
    }
  }

  const fetchDatasets = async () => {
    try {
      const response = await apiFetch('/api/v1/datasets')
      const data = await response.json()
      setDatasets(Array.isArray(data) ? data : [])
    } catch (error) {
      console.error('Error fetching datasets:', error)
    }
  }

  const fetchClusters = async () => {
    try {
      const res = await apiFetch('/api/v1/clusters')
      setClusters(await res.json())
    } catch (error) {
      console.error('Error fetching clusters:', error)
    }
  }

  const fetchTemplates = async () => {
    try {
      const res = await apiFetch('/api/v1/training-templates')
      const data = await res.json()
      setTemplates(Array.isArray(data) ? data : [])
    } catch (error) {
      console.error('Error fetching training templates:', error)
    }
  }

  const clusterNameOf = (id) => clusters.find((c) => c.id === id)?.name || id || '—'

  const applyTemplate = (id) => {
    const tpl = templates.find((t) => t.id === id)
    if (!tpl) {
      setNewJob({ ...newJob, template_id: '' })
      return
    }
    const s = tpl.spec || {}
    setNewJob({
      ...newJob,
      template_id: id,
      image: s.image || '',
      command: s.command || '',
      gpus: s.gpus ?? 1,
      memory: s.memory ?? 32,
      max_runtime: s.max_runtime ?? 0,
      distributed: !!s.distributed,
      framework: s.framework || 'pytorch-ddp',
      replicas: s.replicas ?? 0,
      min_available: s.min_available ?? 0,
    })
  }

  const handleCreateJob = async (e) => {
    e.preventDefault()
    setActionError('')
    try {
      const response = await apiFetch('/api/v1/training-jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newJob),
      })
      if (response.ok) {
        setShowModal(false)
        setNewJob({ ...emptyJob })
        fetchJobs()
        return
      }
      const detail = await response.json().catch(() => ({}))
      setActionError(detail.error || `创建失败（HTTP ${response.status}）`)
    } catch (error) {
      setActionError('创建失败：' + (error.message || error))
    }
  }

  const handleDeleteJob = async (job) => {
    if (!confirm('确定删除该训练任务？集群侧负载会一并清理。')) return
    try {
      await apiFetch(`/api/v1/training-jobs/${job.id}`, { method: 'DELETE' })
      fetchJobs()
    } catch (error) {
      console.error('Error deleting job:', error)
    }
  }

  const runJobAction = async (job, action, confirmText) => {
    if (confirmText && !confirm(confirmText)) return
    setActionError('')
    try {
      const res = await apiFetch(`/api/v1/training-jobs/${job.id}/${action}`, { method: 'POST' })
      if (res.ok) {
        fetchJobs()
        return
      }
      const detail = await res.json().catch(() => ({}))
      setActionError(detail.error || `操作失败（HTTP ${res.status}）`)
    } catch (error) {
      setActionError('操作失败：' + (error.message || error))
    }
  }

  const isTerminal = (job) => ['completed', 'failed', 'cancelled'].includes(job.status)

  const fetchLogs = async (job, query) => {
    setLogLoading(true)
    try {
      const params = new URLSearchParams()
      if (query.pod) params.set('pod', query.pod)
      if (query.task) params.set('task', query.task)
      if (query.tail) params.set('tail', String(query.tail))
      const res = await apiFetch(`/api/v1/training-jobs/${job.id}/logs?${params.toString()}`)
      const data = await res.json().catch(() => ({}))
      if (!res.ok) {
        
        setLogData({
          available: false,
          logs: '',
          pods: data.pods || logData?.pods || [],
          message: data.error || `日志拉取失败（HTTP ${res.status}）`,
        })
        return
      }
      setLogData(data)
    } catch (error) {
      setLogData({ available: false, logs: '', pods: [], message: '日志拉取失败：' + (error.message || error) })
    } finally {
      setLogLoading(false)
    }
  }

  const handleViewLogs = (job) => {
    const query = { pod: '', task: '', tail: 100 }
    setLogJob(job)
    setLogData(null)
    setLogQuery(query)
    fetchLogs(job, query)
  }

  const applyLogQuery = (patch) => {
    const next = { ...logQuery, ...patch }
    if (patch.pod) next.task = ''
    if (patch.task) next.pod = ''
    setLogQuery(next)
    if (logJob) fetchLogs(logJob, next)
  }

  const logTaskOptions = [...new Set((logData?.pods || []).map((p) => p.task).filter(Boolean))]

  const filteredJobs = jobs.filter(job => {
    const matchesSearch = job.name.toLowerCase().includes(searchTerm.toLowerCase())
    const matchesStatus = filterStatus === 'all' || job.status === filterStatus
    return matchesSearch && matchesStatus
  })

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
            <h1 className="text-3xl font-bold text-white mb-2">模型训练</h1>
            <p className="text-slate-400">提交训练任务、管理 checkpoint 与断点续训，并把产物注册为模型版本</p>
          </div>
          <button
            onClick={() => { setActionError(''); setShowModal(true) }}
            className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl"
          >
            <Plus className="w-5 h-5" />
            新建训练
          </button>
        </div>

        {actionError && (
          <div className="mb-4 px-4 py-3 rounded-xl bg-red-500/10 border border-red-500/40 text-red-300 text-sm flex items-start justify-between gap-4">
            <span className="whitespace-pre-wrap">{actionError}</span>
            <button onClick={() => setActionError('')} className="text-red-300/70 hover:text-red-200 leading-none">&times;</button>
          </div>
        )}

        <div className="flex flex-col md:flex-row gap-4 mb-6">
          <div className="relative flex-1">
            <Search className="absolute left-4 top-1/2 transform -translate-y-1/2 text-slate-400 w-5 h-5" />
            <input
              type="text"
              placeholder="搜索任务..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded-xl pl-12 pr-4 py-3 text-white placeholder-slate-400 focus:outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all"
            />
          </div>
          <select
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="bg-slate-800 border border-slate-700 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all"
          >
            <option value="all">全部状态</option>
            <option value="pending">等待中</option>
            <option value="running">运行中</option>
            <option value="retrying">待续训</option>
            <option value="completed">已完成</option>
            <option value="failed">失败</option>
            <option value="cancelled">已取消</option>
          </select>
        </div>

        <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-slate-900/50">
                <tr className="text-left text-slate-400 text-sm border-b border-slate-700">
                  <th className="px-6 py-4 font-medium">任务名称</th>
                  <th className="px-6 py-4 font-medium">状态</th>
                  <th className="px-6 py-4 font-medium">镜像</th>
                  <th className="px-6 py-4 font-medium">GPU</th>
                  <th className="px-6 py-4 font-medium">内存</th>
                  <th className="px-6 py-4 font-medium">Checkpoint</th>
                  <th className="px-6 py-4 font-medium">模型版本</th>
                  <th className="px-6 py-4 font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="text-sm">
                {filteredJobs.map((job) => {
                  const statusStyle = jobStatusStyle(job.status)
                  const StatusIcon = statusStyle.icon
                  const remaining = runtimeRemaining(job, now)
                  return (
                    <tr key={job.id} className="border-b border-slate-700/50 hover:bg-slate-700/30 transition-colors">
                      <td className="px-6 py-4">
                        <div className="font-medium text-white flex items-center gap-2">
                          {job.name}
                          {job.distributed && (
                            <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-purple-500/20 text-purple-400">
                              分布式 ×{(job.replicas || 0) + 1}
                            </span>
                          )}
                          {job.dataset_name && (
                            <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-cyan-500/20 text-cyan-400">
                              {job.dataset_name}
                            </span>
                          )}
                          {job.cluster_id && (
                            <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-500/20 text-slate-300">
                              {clusterNameOf(job.cluster_id)}
                            </span>
                          )}
                        </div>
                        <div className="text-slate-500 text-xs mt-1">{job.command}</div>
                      </td>
                      <td className="px-6 py-4">
                        <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}>
                          <StatusIcon className="w-3.5 h-3.5" />
                          {statusStyle.label}
                        </span>
                        {remaining && (
                          <div
                            className={`mt-1 inline-flex items-center gap-1 text-[10px] ${
                              remaining.overdue
                                ? 'text-red-400'
                                : remaining.leftMs <= 10 * 60000
                                  ? 'text-amber-400'
                                  : 'text-slate-400'
                            }`}
                            title={`超时上限 ${job.max_runtime} 分钟，到期后自动熔断并释放资源`}
                          >
                            <Clock className="w-3 h-3" />
                            {remaining.overdue ? `已超时 ${remaining.text}，即将熔断` : `剩余 ${remaining.text}`}
                          </div>
                        )}
                      </td>
                      <td className="px-6 py-4 text-slate-300 text-xs truncate max-w-xs" title={job.image}>
                        {job.image}
                      </td>
                      <td className="px-6 py-4 text-slate-300">
                        {job.gpus} 张
                        {(job.gpu_memory > 0 || job.gpu_cores > 0) && (
                          <div className="text-[10px] text-cyan-400 mt-0.5">
                            HAMi {job.gpu_memory > 0 ? `${job.gpu_memory}MiB` : ''} {job.gpu_cores > 0 ? `${job.gpu_cores}%` : ''}
                          </div>
                        )}
                      </td>
                      <td className="px-6 py-4 text-slate-300">{job.memory} GB</td>
                      <td className="px-6 py-4">
                        {job.checkpoint_enabled ? (
                          <div className="text-xs">
                            <div className="text-slate-300">
                              {job.latest_checkpoint_step > 0 ? `step ${job.latest_checkpoint_step}` : '尚未落盘'}
                            </div>
                            <div className="text-slate-500 mt-0.5">
                              续训 {job.retry_attempts || 0}/{job.checkpoint_max_retries || 0}
                            </div>
                          </div>
                        ) : (
                          <span className="text-xs text-slate-600">未开启</span>
                        )}
                      </td>
                      <td className="px-6 py-4 text-xs">
                        {job.registered_version_id ? (
                          <span className="px-2 py-1 rounded-full bg-green-500/20 text-green-400" title={`来自任务 ${job.id}`}>
                            {job.register_version_tag || job.registered_version_id}
                          </span>
                        ) : job.register_model_enabled ? (
                          <span className="text-slate-500">待注册</span>
                        ) : (
                          <span className="text-slate-600">—</span>
                        )}
                      </td>
                      <td className="px-6 py-4">
                        <div className="flex items-center gap-2">
                          <button
                            title="取消"
                            disabled={isTerminal(job)}
                            onClick={() => runJobAction(job, 'cancel', '确定取消该训练？集群侧负载会立即停止。')}
                            className="p-2 text-slate-400 hover:text-yellow-400 hover:bg-slate-700 rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                          >
                            <Ban className="w-4 h-4" />
                          </button>
                          <button
                            title="从最近 checkpoint 续训"
                            disabled={job.status !== 'retrying'}
                            onClick={() => runJobAction(job, 'resume')}
                            className="p-2 text-slate-400 hover:text-cyan-400 hover:bg-slate-700 rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                          >
                            <RotateCcw className="w-4 h-4" />
                          </button>
                          <button
                            title="标记完成并注册模型版本"
                            disabled={job.status !== 'running'}
                            onClick={() => runJobAction(job, 'complete', '确认该训练已成功结束？')}
                            className="p-2 text-slate-400 hover:text-green-400 hover:bg-slate-700 rounded-lg transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                          >
                            <CheckCircle2 className="w-4 h-4" />
                          </button>
                          <button title="删除" onClick={() => handleDeleteJob(job)} className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg transition-colors">
                            <Trash2 className="w-4 h-4" />
                          </button>
                          <button title="日志" onClick={() => handleViewLogs(job)} className="p-2 text-slate-400 hover:text-blue-400 hover:bg-slate-700 rounded-lg transition-colors">
                            <FileText className="w-4 h-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {showModal && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-2xl border border-slate-700 shadow-2xl max-h-[90vh] overflow-y-auto">
            <h2 className="text-2xl font-bold text-white mb-6">新建训练任务</h2>
            <form onSubmit={handleCreateJob} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">训练模板</label>
                <select
                  value={newJob.template_id}
                  onChange={(e) => applyTemplate(e.target.value)}
                  className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                >
                  <option value="">不使用模板（手工填写）</option>
                  {templates.map((t) => (
                    <option key={t.id} value={t.id}>{t.name}</option>
                  ))}
                </select>
                {newJob.template_id && (
                  <p className="text-xs text-slate-500 mt-1">
                    {templates.find((t) => t.id === newJob.template_id)?.description}
                  </p>
                )}
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">任务名称</label>
                <input
                  type="text"
                  value={newJob.name}
                  onChange={(e) => setNewJob({ ...newJob, name: e.target.value })}
                  className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">目标集群</label>
                <select
                  value={newJob.cluster_id}
                  onChange={(e) => setNewJob({ ...newJob, cluster_id: e.target.value })}
                  className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                >
                  {clusters.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">镜像</label>
                <input
                  type="text"
                  value={newJob.image}
                  onChange={(e) => setNewJob({ ...newJob, image: e.target.value })}
                  className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                  placeholder="nvcr.io/nvidia/pytorch:23.10-py3"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">启动命令</label>
                <input
                  type="text"
                  value={newJob.command}
                  onChange={(e) => setNewJob({ ...newJob, command: e.target.value })}
                  className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                  placeholder="python train.py"
                  required
                />
              </div>
              <div className="grid grid-cols-3 gap-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">GPU 数量</label>
                  <input
                    type="number"
                    value={newJob.gpus}
                    onChange={(e) => setNewJob({ ...newJob, gpus: parseInt(e.target.value) })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    min="1"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">优先级</label>
                  <input
                    type="number"
                    value={newJob.priority}
                    onChange={(e) => setNewJob({ ...newJob, priority: parseInt(e.target.value) })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    min="1"
                    max="1000"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">超时(分钟)</label>
                  <input
                    type="number"
                    value={newJob.max_runtime}
                    onChange={(e) => setNewJob({ ...newJob, max_runtime: parseInt(e.target.value) || 0 })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    min="0"
                    title="最长运行分钟数，0 表示不限制；超过则自动熔断失败并释放资源"
                  />
                </div>
              </div>

              {}
              <div className="grid grid-cols-2 gap-4 p-4 bg-slate-900/50 rounded-xl border border-slate-700">
                <div className="col-span-2 text-xs text-cyan-400">HAMi GPU 显存/算力隔离（0 表示整卡独占）</div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">显存 (MiB)</label>
                  <input
                    type="number"
                    value={newJob.gpu_memory}
                    onChange={(e) => setNewJob({ ...newJob, gpu_memory: parseInt(e.target.value) })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    min="0"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">算力 (%)</label>
                  <input
                    type="number"
                    value={newJob.gpu_cores}
                    onChange={(e) => setNewJob({ ...newJob, gpu_cores: parseInt(e.target.value) })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    min="0"
                    max="100"
                  />
                </div>
              </div>

              {}
              <div className="p-4 bg-slate-900/50 rounded-xl border border-slate-700 space-y-4">
                <label className="flex items-center gap-3 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={newJob.distributed}
                    onChange={(e) => setNewJob({ ...newJob, distributed: e.target.checked })}
                    className="w-4 h-4 rounded accent-blue-500"
                  />
                  <span className="text-sm font-medium text-slate-300">启用分布式训练（Volcano Gang 调度）</span>
                </label>
                {newJob.distributed && (
                  <div className="grid grid-cols-3 gap-4">
                    <div>
                      <label className="block text-sm font-medium text-slate-300 mb-2">框架</label>
                      <select
                        value={newJob.framework}
                        onChange={(e) => setNewJob({ ...newJob, framework: e.target.value })}
                        className="w-full bg-slate-700 border border-slate-600 rounded-xl px-3 py-3 text-white focus:outline-none focus:border-blue-500"
                      >
                        <option value="pytorch-ddp">PyTorch DDP</option>
                        <option value="deepspeed">DeepSpeed</option>
                        <option value="tensorflow">TensorFlow</option>
                        <option value="mpi">MPI</option>
                      </select>
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-slate-300 mb-2">Worker 数</label>
                      <input
                        type="number"
                        value={newJob.replicas}
                        onChange={(e) => setNewJob({ ...newJob, replicas: parseInt(e.target.value) })}
                        className="w-full bg-slate-700 border border-slate-600 rounded-xl px-3 py-3 text-white focus:outline-none focus:border-blue-500"
                        min="1"
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-slate-300 mb-2">最小可用</label>
                      <input
                        type="number"
                        value={newJob.min_available}
                        onChange={(e) => setNewJob({ ...newJob, min_available: parseInt(e.target.value) })}
                        className="w-full bg-slate-700 border border-slate-600 rounded-xl px-3 py-3 text-white focus:outline-none focus:border-blue-500"
                        min="0"
                      />
                    </div>
                  </div>
                )}
              </div>

              {}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">数据集 (Fluid)</label>
                  <select
                    value={newJob.dataset_name}
                    onChange={(e) => setNewJob({ ...newJob, dataset_name: e.target.value })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                  >
                    <option value="">不挂载</option>
                    {datasets.map((ds) => (
                      <option key={ds.id} value={ds.name}>{ds.name}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">挂载路径</label>
                  <input
                    type="text"
                    value={newJob.mount_path}
                    onChange={(e) => setNewJob({ ...newJob, mount_path: e.target.value })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    placeholder="/data"
                  />
                </div>
              </div>

              {}
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">代码 Commit</label>
                <input
                  type="text"
                  value={newJob.code_commit}
                  onChange={(e) => setNewJob({ ...newJob, code_commit: e.target.value })}
                  className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                  placeholder="例如 a1b2c3d（同一镜像 tag 重复构建时用于区分代码版本）"
                />
              </div>

              {}
              <div className="p-4 bg-slate-900/50 rounded-xl border border-slate-700 space-y-4">
                <label className="flex items-center gap-3 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={newJob.checkpointing.enabled}
                    onChange={(e) => setNewJob({ ...newJob, checkpointing: { ...newJob.checkpointing, enabled: e.target.checked } })}
                    className="w-4 h-4 rounded accent-blue-500"
                  />
                  <span className="text-sm font-medium text-slate-300">启用 Checkpoint（失败后可从断点续训）</span>
                </label>
                {newJob.checkpointing.enabled && (
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="block text-sm font-medium text-slate-300 mb-2">落盘间隔 (steps)</label>
                      <input
                        type="number"
                        value={newJob.checkpointing.interval_steps}
                        onChange={(e) => setNewJob({ ...newJob, checkpointing: { ...newJob.checkpointing, interval_steps: parseInt(e.target.value) || 0 } })}
                        className="w-full bg-slate-700 border border-slate-600 rounded-xl px-3 py-3 text-white focus:outline-none focus:border-blue-500"
                        min="0"
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-slate-300 mb-2">自动续训次数上限</label>
                      <input
                        type="number"
                        value={newJob.checkpointing.max_retries}
                        onChange={(e) => setNewJob({ ...newJob, checkpointing: { ...newJob.checkpointing, max_retries: parseInt(e.target.value) || 0 } })}
                        className="w-full bg-slate-700 border border-slate-600 rounded-xl px-3 py-3 text-white focus:outline-none focus:border-blue-500"
                        min="0"
                        max="20"
                        title="用尽后任务直接判失败，避免无 checkpoint 的任务反复空转烧卡"
                      />
                    </div>
                  </div>
                )}
              </div>

              {}
              <div className="p-4 bg-slate-900/50 rounded-xl border border-slate-700 space-y-4">
                <label className="flex items-center gap-3 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={newJob.register_model.enabled}
                    onChange={(e) => setNewJob({ ...newJob, register_model: { ...newJob.register_model, enabled: e.target.checked } })}
                    className="w-4 h-4 rounded accent-blue-500"
                  />
                  <span className="text-sm font-medium text-slate-300">训练成功后自动注册为模型版本</span>
                </label>
                {newJob.register_model.enabled && (
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="block text-sm font-medium text-slate-300 mb-2">目标模型 ID</label>
                      <input
                        type="text"
                        value={newJob.register_model.model_id}
                        onChange={(e) => setNewJob({ ...newJob, register_model: { ...newJob.register_model, model_id: e.target.value } })}
                        className="w-full bg-slate-700 border border-slate-600 rounded-xl px-3 py-3 text-white focus:outline-none focus:border-blue-500"
                        required
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-slate-300 mb-2">版本号</label>
                      <input
                        type="text"
                        value={newJob.register_model.version_tag}
                        onChange={(e) => setNewJob({ ...newJob, register_model: { ...newJob.register_model, version_tag: e.target.value } })}
                        className="w-full bg-slate-700 border border-slate-600 rounded-xl px-3 py-3 text-white focus:outline-none focus:border-blue-500"
                        placeholder="v1.0.0"
                        required
                      />
                    </div>
                  </div>
                )}
              </div>

              {actionError && (
                <div className="px-4 py-3 rounded-xl bg-red-500/10 border border-red-500/40 text-red-300 text-sm whitespace-pre-wrap">
                  {actionError}
                </div>
              )}

              <div className="flex gap-3 mt-6">
                <button
                  type="button"
                  onClick={() => setShowModal(false)}
                  className="flex-1 bg-slate-700 hover:bg-slate-600 text-white px-6 py-3 rounded-xl font-medium transition-colors"
                >
                  取消
                </button>
                <button
                  type="submit"
                  className="flex-1 flex items-center justify-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all"
                >
                  <Save className="w-4 h-4" />
                  提交训练
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {}
      {logJob && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-800 rounded-2xl p-6 w-full max-w-3xl border border-slate-700 shadow-2xl max-h-[85vh] flex flex-col">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-xl font-bold text-white">训练日志 · {logJob.name}</h2>
              <button onClick={() => setLogJob(null)} className="text-slate-400 hover:text-white text-2xl leading-none">&times;</button>
            </div>

            {}
            <div className="flex flex-wrap items-center gap-2 mb-3 text-sm">
              <select
                value={logQuery.task}
                onChange={(e) => applyLogQuery({ task: e.target.value })}
                className="bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-blue-500"
                title="按 task 角色下钻"
              >
                <option value="">全部角色</option>
                {logTaskOptions.map((t) => (
                  <option key={t} value={t}>{t}</option>
                ))}
              </select>
              <select
                value={logQuery.pod}
                onChange={(e) => applyLogQuery({ pod: e.target.value })}
                className="bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-blue-500 max-w-xs"
                title="按单个副本（Pod）下钻"
              >
                <option value="">全部副本</option>
                {(logData?.pods || []).map((p) => (
                  <option key={p.name} value={p.name}>
                    {p.name}{p.phase ? ` (${p.phase})` : ''}
                  </option>
                ))}
              </select>
              <select
                value={logQuery.tail}
                onChange={(e) => applyLogQuery({ tail: parseInt(e.target.value) })}
                className="bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-blue-500"
                title="每个副本截取的末尾行数"
              >
                {[100, 500, 1000, 5000].map((n) => (
                  <option key={n} value={n}>末尾 {n} 行</option>
                ))}
              </select>
              <button
                onClick={() => fetchLogs(logJob, logQuery)}
                disabled={logLoading}
                className="flex items-center gap-1.5 bg-slate-700 hover:bg-slate-600 text-white px-3 py-2 rounded-lg transition-colors disabled:opacity-40"
              >
                <RefreshCw className={`w-4 h-4 ${logLoading ? 'animate-spin' : ''}`} />
                刷新
              </button>
              <span className="text-xs text-slate-500">
                {(logData?.pods || []).length > 0 ? `共 ${logData.pods.length} 个副本` : ''}
              </span>
            </div>

            <div className="flex-1 overflow-auto bg-slate-900 rounded-xl p-4 border border-slate-700">
              {logLoading && <div className="text-slate-400 text-sm">加载中...</div>}
              {!logLoading && logData && !logData.available && (
                <div className="text-amber-400 text-sm whitespace-pre-wrap">
                  {logData.message || '该任务暂无可拉取的日志。'}
                  {logData.failure_reason ? `\n\n失败原因：${logData.failure_reason}` : ''}
                </div>
              )}
              {!logLoading && logData && logData.available && (
                <pre className="text-green-400 text-xs whitespace-pre-wrap font-mono">{logData.logs || '（日志为空）'}</pre>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default Training
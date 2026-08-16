import React, { useState } from 'react'
import { apiJson } from '../auth'
import { usePolling } from '../utils/usePolling'
import { LineChart, Line, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, BarChart, Bar, Cell } from 'recharts'
import { Cpu, HardDrive, Activity, Clock, CheckCircle, AlertCircle, Briefcase, MonitorSmartphone } from 'lucide-react'

const Dashboard = () => {
  const [metrics, setMetrics] = useState(null)
  const [resources, setResources] = useState([])
  const [jobs, setJobs] = useState([])
  const [loading, setLoading] = useState(true)

  const fetchData = async () => {
    try {
      const [metricsData, resourcesData, jobsData] = await Promise.all([
        apiJson('/metrics'),
        apiJson('/resources'),
        apiJson('/training-jobs'),
      ])
      setMetrics(metricsData)
      setResources(resourcesData)
      setJobs(jobsData)
    } catch (error) {
      
      console.error('Error fetching data:', error)
    } finally {
      setLoading(false)
    }
  }

  usePolling(fetchData)

  const chartData = [
    { name: '00:00', gpu: 45, memory: 38 },
    { name: '04:00', gpu: 52, memory: 45 },
    { name: '08:00', gpu: 78, memory: 65 },
    { name: '12:00', gpu: 85, memory: 72 },
    { name: '16:00', gpu: 92, memory: 80 },
    { name: '20:00', gpu: 75, memory: 68 },
    { name: '现在', gpu: metrics?.gpu_utilization || 68, memory: metrics?.memory_utilization || 55 },
  ]

  const COLORS = ['#3b82f6', '#8b5cf6', '#06b6d4', '#10b981', '#f59e0b']

  const jobStatusData = [
    { name: '运行中', value: jobs.filter(j => j.status === 'running').length, color: '#10b981' },
    { name: '等待中', value: jobs.filter(j => j.status === 'pending').length, color: '#f59e0b' },
    { name: '待续训', value: jobs.filter(j => j.status === 'retrying').length, color: '#06b6d4' },
    { name: '已完成', value: jobs.filter(j => j.status === 'completed').length, color: '#3b82f6' },
    { name: '失败', value: jobs.filter(j => j.status === 'failed').length, color: '#ef4444' },
  ]

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
        <div className="mb-8">
          <h1 className="text-3xl font-bold text-white mb-2">仪表盘</h1>
          <p className="text-slate-400">AI 算力调度系统概览</p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
          <div className="bg-gradient-to-br from-slate-800 to-slate-900 rounded-2xl p-6 border border-slate-700 shadow-xl">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 bg-blue-500/20 rounded-xl flex items-center justify-center">
                <Cpu className="w-6 h-6 text-blue-400" />
              </div>
              <span className="text-xs font-medium text-green-400 bg-green-500/20 px-2 py-1 rounded-full">
                +12%
              </span>
            </div>
            <h3 className="text-slate-400 text-sm font-medium mb-1">GPU 总数</h3>
            <p className="text-3xl font-bold text-white">{metrics?.total_gpus || 0}</p>
            <p className="text-xs text-slate-500 mt-2">
              可用: <span className="text-green-400">{metrics?.available_gpus || 0}</span>
            </p>
          </div>

          <div className="bg-gradient-to-br from-slate-800 to-slate-900 rounded-2xl p-6 border border-slate-700 shadow-xl">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 bg-purple-500/20 rounded-xl flex items-center justify-center">
                <Activity className="w-6 h-6 text-purple-400" />
              </div>
              <span className="text-xs font-medium text-green-400 bg-green-500/20 px-2 py-1 rounded-full">
                活跃
              </span>
            </div>
            <h3 className="text-slate-400 text-sm font-medium mb-1">GPU 利用率</h3>
            <p className="text-3xl font-bold text-white">{metrics?.gpu_utilization || 0}%</p>
            <div className="w-full bg-slate-700 rounded-full h-2 mt-3">
              <div
                className="bg-gradient-to-r from-purple-500 to-pink-500 h-2 rounded-full"
                style={{ width: `${metrics?.gpu_utilization || 0}%` }}
              ></div>
            </div>
          </div>

          <div className="bg-gradient-to-br from-slate-800 to-slate-900 rounded-2xl p-6 border border-slate-700 shadow-xl">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 bg-cyan-500/20 rounded-xl flex items-center justify-center">
                <HardDrive className="w-6 h-6 text-cyan-400" />
              </div>
              <span className="text-xs font-medium text-blue-400 bg-blue-500/20 px-2 py-1 rounded-full">
                {metrics?.total_memory || 0}GB
              </span>
            </div>
            <h3 className="text-slate-400 text-sm font-medium mb-1">显存使用</h3>
            <p className="text-3xl font-bold text-white">{metrics?.used_memory || 0}GB</p>
            <p className="text-xs text-slate-500 mt-2">
              利用率: <span className="text-cyan-400">{metrics?.memory_utilization || 0}%</span>
            </p>
          </div>

          <div className="bg-gradient-to-br from-slate-800 to-slate-900 rounded-2xl p-6 border border-slate-700 shadow-xl">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 bg-green-500/20 rounded-xl flex items-center justify-center">
                <Briefcase className="w-6 h-6 text-green-400" />
              </div>
              <span className="text-xs font-medium text-yellow-400 bg-yellow-500/20 px-2 py-1 rounded-full">
                {metrics?.pending_jobs || 0} 等待
              </span>
            </div>
            <h3 className="text-slate-400 text-sm font-medium mb-1">任务总数</h3>
            <p className="text-3xl font-bold text-white">{metrics?.total_jobs || 0}</p>
            <p className="text-xs text-slate-500 mt-2">
              运行中: <span className="text-green-400">{metrics?.running_jobs || 0}</span>
            </p>
          </div>

          <div className="bg-gradient-to-br from-slate-800 to-slate-900 rounded-2xl p-6 border border-slate-700 shadow-xl">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 bg-orange-500/20 rounded-xl flex items-center justify-center">
                <MonitorSmartphone className="w-6 h-6 text-orange-400" />
              </div>
              <span className="text-xs font-medium text-cyan-400 bg-cyan-500/20 px-2 py-1 rounded-full">
                {metrics?.workspace_running || 0} 运行
              </span>
            </div>
            <h3 className="text-slate-400 text-sm font-medium mb-1">工作空间</h3>
            <p className="text-3xl font-bold text-white">{metrics?.workspace_total || 0}</p>
            <p className="text-xs text-slate-500 mt-2">
              已停止: <span className="text-slate-400">{metrics?.workspace_by_status?.stopped || 0}</span>
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
          <div className="bg-slate-800 rounded-2xl p-6 border border-slate-700 shadow-xl">
            <h3 className="text-lg font-semibold text-white mb-6">资源使用趋势</h3>
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={chartData}>
                  <defs>
                    <linearGradient id="colorGpu" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3}/>
                      <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
                  <XAxis dataKey="name" stroke="#94a3b8" fontSize={12} />
                  <YAxis stroke="#94a3b8" fontSize={12} />
                  <Tooltip
                    contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #334155', borderRadius: '8px' }}
                    itemStyle={{ color: '#f1f5f9' }}
                  />
                  <Area type="monotone" dataKey="gpu" stroke="#3b82f6" fillOpacity={1} fill="url(#colorGpu)" />
                  <Area type="monotone" dataKey="memory" stroke="#8b5cf6" fillOpacity={0.3} fill="#8b5cf6" />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </div>

          <div className="bg-slate-800 rounded-2xl p-6 border border-slate-700 shadow-xl">
            <h3 className="text-lg font-semibold text-white mb-6">训练任务状态分布</h3>
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={jobStatusData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
                  <XAxis dataKey="name" stroke="#94a3b8" fontSize={12} />
                  <YAxis stroke="#94a3b8" fontSize={12} />
                  <Tooltip
                    contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #334155', borderRadius: '8px' }}
                    itemStyle={{ color: '#f1f5f9' }}
                  />
                  <Bar dataKey="value" radius={[8, 8, 0, 0]}>
                    {jobStatusData.map((entry, index) => (
                      <Cell key={`cell-${index}`} fill={entry.color} />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>

        <div className="bg-slate-800 rounded-2xl p-6 border border-slate-700 shadow-xl">
          <h3 className="text-lg font-semibold text-white mb-6">最近训练任务</h3>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="text-left text-slate-400 text-sm border-b border-slate-700">
                  <th className="pb-4 font-medium">任务名称</th>
                  <th className="pb-4 font-medium">规格</th>
                  <th className="pb-4 font-medium">状态</th>
                  <th className="pb-4 font-medium">GPU</th>
                  <th className="pb-4 font-medium">优先级</th>
                </tr>
              </thead>
              <tbody className="text-sm">
                {jobs.slice(0, 5).map((job) => (
                  <tr key={job.id} className="border-b border-slate-700/50 hover:bg-slate-700/30">
                    <td className="py-4 text-white font-medium">{job.name}</td>
                    <td className="py-4">
                      <span className="px-2 py-1 rounded-full text-xs font-medium bg-blue-500/20 text-blue-400">
                        {job.distributed ? `${job.framework || '分布式'} ×${(job.replicas || 0) + 1}` : '单机'}
                      </span>
                    </td>
                    <td className="py-4">
                      <span className={`inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs font-medium ${
                        job.status === 'running'
                          ? 'bg-green-500/20 text-green-400'
                          : job.status === 'pending'
                          ? 'bg-yellow-500/20 text-yellow-400'
                          : job.status === 'failed'
                          ? 'bg-red-500/20 text-red-400'
                          : job.status === 'retrying'
                          ? 'bg-cyan-500/20 text-cyan-400'
                          : 'bg-blue-500/20 text-blue-400'
                      }`}>
                        {job.status === 'running' ? (
                          <><CheckCircle className="w-3 h-3" />
                          运行中
                        </>
                        ) : job.status === 'pending' ? (
                          <><Clock className="w-3 h-3" />
                          等待中
                        </>
                        ) : (
                          
                          { retrying: '待续训', completed: '已完成', failed: '失败', cancelled: '已取消' }[job.status] || job.status
                        )}
                      </span>
                    </td>
                    <td className="py-4 text-slate-300">{job.gpus} 张</td>
                    <td className="py-4 text-slate-300">{job.priority}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  )
}

export default Dashboard
import React, { useState, useEffect } from 'react'
import { apiFetch } from '../auth'
import { Activity, Cpu, Layers, Gauge, ExternalLink, RefreshCw, AlertTriangle, LayoutGrid } from 'lucide-react'

const DASHBOARDS = [
  { uid: 'fuze-ai-overview', title: '总览', icon: LayoutGrid },
  { uid: 'fuze-gpu', title: 'GPU 资源', icon: Cpu },
  { uid: 'fuze-jobs', title: '任务', icon: Activity },
  { uid: 'fuze-latency-error', title: '延迟/错误率', icon: Gauge },
  { uid: 'fuze-quota-slo', title: '配额/SLO', icon: Layers },
  { uid: 'fuze-alerts', title: '告警', icon: AlertTriangle },
]

const grafanaUrl = (uid) => `/grafana/d/${uid}/${uid}?orgId=1&kiosk`

const Monitoring = () => {
  const [metrics, setMetrics] = useState(null)
  const [active, setActive] = useState([])
  const [activeTab, setActiveTab] = useState(DASHBOARDS[0].uid)
  const [updatedAt, setUpdatedAt] = useState(null)

  const fetchMetrics = async () => {
    try {
      const res = await apiFetch('/api/v1/metrics')
      const data = await res.json()
      setMetrics(data)
      setUpdatedAt(new Date())
    } catch (e) {
      console.error('Error fetching metrics:', e)
    }
  }

  const fetchActive = async () => {
    try {
      const a = await apiFetch('/api/v1/alerts/active').then((x) => x.json()).catch(() => [])
      setActive(Array.isArray(a) ? a : [])
    } catch (e) {  }
  }

  useEffect(() => {
    fetchMetrics()
    fetchActive()
    const timer = setInterval(() => { fetchMetrics(); fetchActive() }, 10000)
    return () => clearInterval(timer)
  }, [])

  const firing = active.filter((a) => a.state === 'firing')
  const cards = metrics ? [
    { icon: Gauge, label: 'GPU 利用率', value: `${metrics.gpu_utilization}%`, sub: `${metrics.used_gpus}/${metrics.total_gpus} 卡`, color: 'from-blue-500 to-cyan-500' },
    { icon: Layers, label: '显存利用率', value: `${metrics.memory_utilization}%`, sub: `${metrics.used_memory}/${metrics.total_memory} GB`, color: 'from-purple-500 to-pink-500' },
    { icon: Activity, label: '运行中任务', value: metrics.running_jobs, sub: `等待 ${metrics.pending_jobs} · 完成 ${metrics.completed_jobs}`, color: 'from-green-500 to-emerald-500' },
    { icon: Cpu, label: '任务总数', value: metrics.total_jobs, sub: '全部提交任务', color: 'from-orange-500 to-red-500' },
  ] : []

  return (
    <div className="p-8 bg-gradient-to-br from-slate-900 to-slate-800 min-h-screen">
      <div className="max-w-7xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-white mb-2">监控中心</h1>
            <p className="text-slate-400">Prometheus 指标采集 + Grafana 可视化大盘</p>
          </div>
          <div className="flex items-center gap-3">
            {firing.length > 0 && (
              <a href="/alerts" className="flex items-center gap-2 bg-red-500/20 text-red-400 border border-red-500/40 hover:bg-red-500/30 px-4 py-2.5 rounded-xl text-sm font-medium transition-all">
                <AlertTriangle className="w-4 h-4" /> {firing.length} 告警触发
              </a>
            )}
            {updatedAt && (
              <span className="text-xs text-slate-500 flex items-center gap-1">
                <RefreshCw className="w-3.5 h-3.5" /> {updatedAt.toLocaleTimeString()}
              </span>
            )}
            <a href={grafanaUrl(activeTab)} target="_blank" rel="noreferrer" className="flex items-center gap-2 bg-gradient-to-r from-orange-500 to-red-500 hover:from-orange-600 hover:to-red-600 text-white px-5 py-2.5 rounded-xl font-medium transition-all shadow-lg">
              <ExternalLink className="w-4 h-4" /> 打开 Grafana
            </a>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-6 mb-8">
          {cards.map((c, i) => {
            const Icon = c.icon
            return (
              <div key={i} className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6">
                <div className="flex items-center justify-between mb-4">
                  <div className={`w-11 h-11 bg-gradient-to-br ${c.color} rounded-xl flex items-center justify-center`}>
                    <Icon className="w-6 h-6 text-white" />
                  </div>
                </div>
                <div className="text-3xl font-bold text-white mb-1">{c.value}</div>
                <div className="text-sm text-slate-400">{c.label}</div>
                <div className="text-xs text-slate-500 mt-2">{c.sub}</div>
              </div>
            )
          })}
          {!metrics && <div className="col-span-full text-center text-slate-500 py-8">加载指标中...</div>}
        </div>

        <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-4 mb-6">
          <div className="flex items-center justify-between px-2 py-2 mb-2">
            <h2 className="text-lg font-semibold text-white">Grafana 大盘</h2>
            <span className="text-xs text-slate-500">数据源：Prometheus（抓取后端 /metrics 端点）</span>
          </div>
          <div className="flex flex-wrap gap-2 mb-3">
            {DASHBOARDS.map((d) => {
              const Icon = d.icon
              return (
                <button
                  key={d.uid}
                  onClick={() => setActiveTab(d.uid)}
                  className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                    activeTab === d.uid ? 'bg-blue-600 text-white' : 'bg-slate-700/60 text-slate-300 hover:bg-slate-600'
                  }`}
                >
                  <Icon className="w-4 h-4" /> {d.title}
                </button>
              )
            })}
          </div>
          <div className="rounded-xl overflow-hidden border border-slate-700 bg-slate-900">
            <iframe
              key={activeTab}
              title="Grafana Dashboard"
              src={grafanaUrl(activeTab)}
              className="w-full"
              style={{ height: '600px', border: 'none' }}
            />
          </div>
          <p className="text-xs text-slate-500 px-2 py-2">
            若面板未显示，请确认已部署监控栈（k8s/monitoring）并通过 Ingress 暴露 Grafana 到 <code className="text-slate-400">/grafana</code>。
          </p>
        </div>

        {firing.length > 0 && (
          <div className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-4 mb-6">
            <div className="flex items-center gap-2 mb-3 px-2">
              <AlertTriangle className="w-4 h-4 text-red-400" />
              <h2 className="text-sm font-semibold text-white">触发中的告警（实时）</h2>
            </div>
            <div className="space-y-2">
              {firing.slice(0, 5).map((a) => (
                <div key={a.fingerprint} className="flex items-center justify-between bg-slate-900/60 rounded-lg px-4 py-2">
                  <div className="text-sm text-white font-medium">{a.rule_name || a.labels?.alertname}</div>
                  <div className="text-xs text-slate-400">{a.annotations?.summary || a.annotations?.description || '-'}</div>
                  <span className={`text-xs px-2 py-0.5 rounded-full border ${a.severity === 'critical' ? 'bg-red-500/20 text-red-400 border-red-500/40' : a.severity === 'warning' ? 'bg-amber-500/20 text-amber-400 border-amber-500/40' : 'bg-sky-500/20 text-sky-400 border-sky-500/40'}`}>{a.severity}</span>
                </div>
              ))}
            </div>
            <a href="/alerts" className="inline-block mt-3 text-xs text-blue-400 hover:text-blue-300">查看全部 →</a>
          </div>
        )}
      </div>
    </div>
  )
}

export default Monitoring
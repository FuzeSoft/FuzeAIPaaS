import React from 'react'
import { Link, useLocation } from 'react-router-dom'
import { LayoutDashboard, Cpu, Briefcase, Server, Zap, Rocket, Database, LineChart, Bell, Boxes, FlaskConical, ClipboardCheck, BrainCircuit, MonitorSmartphone, Settings as SettingsIcon, KeyRound, Workflow, Wrench, Sparkles, Gauge } from 'lucide-react'
import { getUser, isPlatformAdmin } from '../auth'

const Sidebar = ({ onLogout }) => {
  const location = useLocation()
  const user = getUser()

  const menuGroups = [
    {
      title: '推理优先',
      items: [
        { path: '/models', icon: Boxes, label: '模型仓库' },
        { path: '/inference', icon: Rocket, label: '推理服务' },
        { path: '/inference-accel', icon: Gauge, label: '推理加速' },
        { path: '/workspaces', icon: MonitorSmartphone, label: '工作空间' },
        { path: '/llmops', icon: BrainCircuit, label: 'LLMOps' },
        { path: '/agents', icon: Workflow, label: 'Agent 编排' },
        { path: '/edge', icon: Server, label: '边缘部署' },
        { path: '/tools', icon: Wrench, label: '工具注册表' },
      ],
    },
    {
      title: '算力与底座',
      items: [
        { path: '/dashboard', icon: LayoutDashboard, label: '仪表盘' },
        { path: '/clusters', icon: Server, label: '集群管理' },
        { path: '/resources', icon: Cpu, label: '资源管理' },
        { path: '/training', icon: Briefcase, label: '模型训练' },
        { path: '/experiments', icon: FlaskConical, label: '实验管理' },
        { path: '/automl', icon: Sparkles, label: 'AutoML / NAS' },
        { path: '/evaluations', icon: ClipboardCheck, label: '评估管理' },
        { path: '/datasets', icon: Database, label: '数据集' },
        { path: '/monitoring', icon: LineChart, label: '监控中心' },
        ...(isPlatformAdmin() || user?.role === 'tenant_admin'
          ? [{ path: '/alerts', icon: Bell, label: '告警中心' }]
          : []),
      ],
    },
    {
      title: '账户与安全',
      items: [
        { path: '/settings', icon: SettingsIcon, label: '账户设置' },
        ...(isPlatformAdmin()
          ? [{ path: '/admin/idps', icon: KeyRound, label: '身份源 (IdP)' }]
          : []),
      ],
    },
  ]

  return (
    <div className="fixed left-0 top-0 h-screen w-64 bg-slate-900/95 border-r border-slate-800 backdrop-blur-sm z-50 flex flex-col">
      <div className="p-6 border-b border-slate-800">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-xl flex items-center justify-center">
            <Zap className="w-6 h-6 text-white" />
          </div>
          <div>
            <h1 className="text-xl font-bold text-white">Fuze AI</h1>
            <p className="text-xs text-slate-400">推理优先 · 信创底座</p>
          </div>
        </div>
      </div>
      <nav className="p-4 flex-1 overflow-y-auto">
        {menuGroups.map((group) => (
          <div key={group.title} className="mb-4">
            <div className="px-4 mb-2 text-[11px] font-semibold uppercase tracking-wider text-slate-500">{group.title}</div>
            <ul className="space-y-1">
              {group.items.map((item) => {
                const Icon = item.icon
                
                const isActive =
                  location.pathname === item.path || location.pathname.startsWith(`${item.path}/`)
                return (
                  <li key={item.path}>
                    <Link
                      to={item.path}
                      className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-all duration-200 ${
                        isActive
                          ? 'bg-gradient-to-r from-blue-600 to-purple-600 text-white shadow-lg shadow-blue-500/25'
                          : 'text-slate-400 hover:bg-slate-800 hover:text-white'
                      }`}
                    >
                      <Icon className="w-5 h-5" />
                      <span className="font-medium">{item.label}</span>
                    </Link>
                  </li>
                )
              })}
            </ul>
          </div>
        ))}
      </nav>
      <div className="p-4 border-t border-slate-800">
        <div className="bg-slate-800/50 rounded-lg p-4">
          <div className="flex items-center justify-between mb-2">
            <p className="text-xs text-slate-400">当前用户</p>
            <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" title="运行正常"></div>
          </div>
          <p className="text-sm text-slate-200 font-medium truncate">
            {user ? user.username : 'system-admin'}
          </p>
          <p className="text-xs text-slate-400 mb-3">{user ? user.role : 'platform_admin'}</p>
          <button
            onClick={onLogout}
            className="w-full text-xs font-medium text-slate-300 bg-slate-700/60 hover:bg-slate-600 rounded-md py-2 transition-colors"
          >
            退出登录
          </button>
        </div>
      </div>
    </div>
  )
}

export default Sidebar
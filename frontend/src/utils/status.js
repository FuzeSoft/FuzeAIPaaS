import { CheckCircle, Clock, AlertCircle, PauseCircle, RotateCcw } from 'lucide-react'

export const jobStatusStyle = (status) => {
  switch (status) {
    case 'running': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '运行中' }
    case 'pending': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', icon: Clock, label: '等待中' }
    case 'paused': return { bg: 'bg-orange-500/20', text: 'text-orange-400', icon: PauseCircle, label: '已暂停' }
    
    case 'retrying': return { bg: 'bg-cyan-500/20', text: 'text-cyan-400', icon: RotateCcw, label: '待续训' }
    case 'completed': return { bg: 'bg-blue-500/20', text: 'text-blue-400', icon: CheckCircle, label: '已完成' }
    case 'failed': return { bg: 'bg-red-500/20', text: 'text-red-400', icon: AlertCircle, label: '失败' }
    case 'cancelled': return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: '已取消' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: status }
  }
}

export const inferenceStatusStyle = (status) => {
  switch (status) {
    case 'ready': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '就绪' }
    case 'pending': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', icon: Clock, label: '部署中' }
    case 'failed': return { bg: 'bg-red-500/20', text: 'text-red-400', icon: AlertCircle, label: '失败' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: status }
  }
}

export const modelVersionStatusStyle = (status) => {
  switch (status) {
    case 'new': return { bg: 'bg-blue-500/20', text: 'text-blue-400', icon: Clock, label: '未构建' }
    case 'ready': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '就绪' }
    case 'building': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', icon: Clock, label: '构建中' }
    case 'failed': return { bg: 'bg-red-500/20', text: 'text-red-400', icon: AlertCircle, label: '失败' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: status }
  }
}

export const resourceStatusStyle = (status) => {
  switch (status) {
    case 'available': return { bg: 'bg-green-500/20', text: 'text-green-400', label: '可用' }
    case 'allocated': return { bg: 'bg-blue-500/20', text: 'text-blue-400', label: '已分配' }
    case 'maintenance': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', label: '维护中' }
    default: return { bg: 'bg-red-500/20', text: 'text-red-400', label: '故障' }
  }
}

export const datasetStatusStyle = (status) => {
  switch (status) {
    case 'bound': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '已就绪' }
    case 'pending': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', icon: Clock, label: '绑定中' }
    case 'failed': return { bg: 'bg-red-500/20', text: 'text-red-400', icon: AlertCircle, label: '失败' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: status }
  }
}

export const runStatusStyle = (status) => {
  switch (status) {
    case 'running': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '运行中' }
    case 'pending': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', icon: Clock, label: '等待中' }
    case 'completed': return { bg: 'bg-blue-500/20', text: 'text-blue-400', icon: CheckCircle, label: '已完成' }
    case 'failed': return { bg: 'bg-red-500/20', text: 'text-red-400', icon: AlertCircle, label: '失败' }
    case 'cancelled': return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: '已取消' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: status || '未知' }
  }
}

export const experimentStatusStyle = (status) => {
  switch (status) {
    case 'active': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '进行中' }
    case 'archived': return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: PauseCircle, label: '已归档' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: status || '未知' }
  }
}

export const clusterStatusClass = (status) => {
  switch (status) {
    case 'healthy': return 'status-badge status-healthy'
    case 'unhealthy': return 'status-badge status-offline'
    default: return 'status-badge status-pending'
  }
}

export const verdictStyle = (verdict) => {
  switch (verdict) {
    case 'pass': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '通过' }
    case 'fail': return { bg: 'bg-red-500/20', text: 'text-red-400', icon: AlertCircle, label: '不通过' }
    case 'pending': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', icon: Clock, label: '待裁决' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: verdict || '未知' }
  }
}

export const workspaceStatusStyle = (status) => {
  switch (status) {
    case 'running': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '运行中' }
    case 'starting': return { bg: 'bg-cyan-500/20', text: 'text-cyan-400', icon: Clock, label: '启动中' }
    case 'stopping': return { bg: 'bg-orange-500/20', text: 'text-orange-400', icon: RotateCcw, label: '停止中' }
    case 'stopped': return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: PauseCircle, label: '已停止' }
    case 'pending': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', icon: Clock, label: '待启动' }
    case 'failed': return { bg: 'bg-red-500/20', text: 'text-red-400', icon: AlertCircle, label: '失败' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: status || '未知' }
  }
}

export const judgeModeStyle = (mode) => {
  switch (mode) {
    case 'threshold': case '': return { bg: 'bg-blue-500/20', text: 'text-blue-400', label: '阈值裁决' }
    case 'human': return { bg: 'bg-purple-500/20', text: 'text-purple-400', label: '人工评审' }
    case 'llm': return { bg: 'bg-cyan-500/20', text: 'text-cyan-400', label: 'LLM 评审' }
    case 'hybrid': return { bg: 'bg-amber-500/20', text: 'text-amber-400', label: '混合评审' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', label: mode || '未知' }
  }
}

export const compressionStatusStyle = (status) => {
  switch (status) {
    case 'pending': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', icon: Clock, label: '等待中' }
    case 'running': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '压缩中' }
    case 'succeeded': return { bg: 'bg-blue-500/20', text: 'text-blue-400', icon: CheckCircle, label: '已完成' }
    case 'failed': return { bg: 'bg-red-500/20', text: 'text-red-400', icon: AlertCircle, label: '失败' }
    case 'cancelled': return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: PauseCircle, label: '已取消' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', icon: Clock, label: status || '未知' }
  }
}

export const compressionTypeStyle = (type) => {
  switch (type) {
    case 'quantize': return { bg: 'bg-cyan-500/20', text: 'text-cyan-400', label: '量化' }
    case 'prune': return { bg: 'bg-purple-500/20', text: 'text-purple-400', label: '剪枝' }
    case 'distill': return { bg: 'bg-pink-500/20', text: 'text-pink-400', label: '蒸馏' }
    case 'convert': return { bg: 'bg-amber-500/20', text: 'text-amber-400', label: '格式转换' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', label: type || '未知' }
  }
}

export const backendStyle = (backend) => {
  switch (backend) {
    case 'pytorch': return { bg: 'bg-orange-500/20', text: 'text-orange-400', label: 'PyTorch' }
    case 'onnxruntime': return { bg: 'bg-teal-500/20', text: 'text-teal-400', label: 'ONNX Runtime' }
    case 'openvino': return { bg: 'bg-indigo-500/20', text: 'text-indigo-400', label: 'OpenVINO' }
    default: return { bg: 'bg-slate-500/20', text: 'text-slate-400', label: backend || '未知' }
  }
}

export const reproductionStateStyle = (state) => {
  switch (state) {
    case 'matched': return { bg: 'bg-green-500/20', text: 'text-green-400', icon: CheckCircle, label: '可复现' }
    case 'diverged': return { bg: 'bg-red-500/20', text: 'text-red-400', icon: AlertCircle, label: '偏差过大' }
    case 'pending': return { bg: 'bg-yellow-500/20', text: 'text-yellow-400', icon: Clock, label: '复现中' }
    default: return null
  }
}
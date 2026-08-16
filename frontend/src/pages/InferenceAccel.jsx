import React, { useState, useEffect } from 'react'
import { apiFetch } from '../auth'
import {
  compressionStatusStyle,
  compressionTypeStyle,
  backendStyle,
} from '../utils/status'
import { Plus, Zap, Trash2, ArrowRight, Gauge, Boxes } from 'lucide-react'

const InferenceAccel = () => {
  const [tasks, setTasks] = useState([])
  const [loading, setLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [filter, setFilter] = useState('all')
  const [form, setForm] = useState({
    name: '',
    type: 'quantize',
    backend: 'pytorch',
    model_version_id: '',
    orig_accuracy: '',
    gate_threshold: '0.01',
    config: '',
  })

  const defaultConfig = (type) => {
    switch (type) {
      case 'quantize':
        return JSON.stringify({ method: 'dynamic', bits: 8 }, null, 2)
      case 'prune':
        return JSON.stringify({ strategy: 'structured', sparsity: 0.5 }, null, 2)
      case 'distill':
        return JSON.stringify({ teacher_model_uri: 'model-xxx', temperature: 2, alpha: 0.5 }, null, 2)
      case 'convert':
        return JSON.stringify({ target_format: 'onnx' }, null, 2)
      default:
        return '{}'
    }
  }

  useEffect(() => {
    fetchTasks()
  }, [])

  const fetchTasks = async () => {
    try {
      const res = await apiFetch('/api/v1/optimize/tasks')
      const data = await res.json()
      setTasks(Array.isArray(data.tasks) ? data.tasks : [])
    } catch (e) {
      console.error('Error fetching compression tasks:', e)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (e) => {
    e.preventDefault()
    const payload = {
      name: form.name,
      type: form.type,
      backend: form.backend,
      model_version_id: form.model_version_id,
      config: form.config,
    }
    if (form.orig_accuracy !== '') payload.orig_accuracy = Number(form.orig_accuracy)
    if (form.gate_threshold !== '') payload.gate_threshold = Number(form.gate_threshold)
    try {
      const res = await apiFetch('/api/v1/optimize/tasks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (res.ok) {
        setShowModal(false)
        setForm((f) => ({ ...f, name: '', model_version_id: '', config: defaultConfig(f.type) }))
        fetchTasks()
      } else {
        const err = await res.json().catch(() => ({}))
        alert('创建压缩任务失败：' + (err.error || res.statusText))
      }
    } catch (e) {
      console.error('Error creating compression task:', e)
      alert('创建压缩任务失败：网络错误')
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除该压缩任务？')) return
    try {
      await apiFetch(`/api/v1/optimize/tasks/${id}`, { method: 'DELETE' })
      fetchTasks()
    } catch (e) {
      console.error('Error deleting compression task:', e)
    }
  }

  const handleCancel = async (id) => {
    try {
      await apiFetch(`/api/v1/optimize/tasks/${id}/cancel`, { method: 'POST' })
      fetchTasks()
    } catch (e) {
      console.error('Error cancelling compression task:', e)
    }
  }

  const filtered = tasks.filter((t) => filter === 'all' || t.type === filter)

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
            <h1 className="text-3xl font-bold text-white mb-2">推理加速</h1>
            <p className="text-slate-400">量化 / 剪枝 / 蒸馏 / 格式转换（TensorRT · ONNX · OpenVINO），带精度门禁</p>
          </div>
          <button
            onClick={() => {
              setForm((f) => ({ ...f, config: defaultConfig(f.type) }))
              setShowModal(true)
            }}
            className="flex items-center gap-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all shadow-lg hover:shadow-xl"
          >
            <Plus className="w-5 h-5" />
            创建加速任务
          </button>
        </div>

        {}
        <div className="flex flex-wrap gap-2 mb-6">
          {[
            { key: 'all', label: '全部' },
            { key: 'quantize', label: '量化' },
            { key: 'prune', label: '剪枝' },
            { key: 'distill', label: '蒸馏' },
            { key: 'convert', label: '格式转换' },
          ].map((f) => (
            <button
              key={f.key}
              onClick={() => setFilter(f.key)}
              className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                filter === f.key
                  ? 'bg-gradient-to-r from-blue-600 to-purple-600 text-white shadow-lg'
                  : 'bg-slate-800 text-slate-400 hover:bg-slate-700 hover:text-white'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {filtered.map((t) => {
            const statusStyle = compressionStatusStyle(t.status)
            const typeStyle = compressionTypeStyle(t.type)
            const backend = backendStyle(t.backend)
            const StatusIcon = statusStyle.icon
            const done = t.status === 'succeeded'
            const gateFailed = t.status === 'failed' && t.gatePass === false
            return (
              <div
                key={t.id}
                className="bg-slate-800 rounded-2xl border border-slate-700 shadow-xl p-6 hover:border-blue-500/50 transition-all"
              >
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="w-10 h-10 bg-gradient-to-br from-cyan-500 to-blue-600 rounded-xl flex items-center justify-center shrink-0">
                      <Zap className="w-5 h-5 text-white" />
                    </div>
                    <div className="min-w-0">
                      <h3 className="font-bold text-white truncate" title={t.name}>{t.name}</h3>
                      <div className="flex items-center gap-2 mt-1 flex-wrap">
                        <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full ${statusStyle.bg} ${statusStyle.text}`}>
                          {StatusIcon && <StatusIcon className="w-3 h-3" />}
                          {statusStyle.label}
                        </span>
                        <span className={`inline-flex items-center text-xs px-2 py-0.5 rounded-full ${typeStyle.bg} ${typeStyle.text}`}>
                          {typeStyle.label}
                        </span>
                        <span className={`inline-flex items-center text-xs px-2 py-0.5 rounded-full ${backend.bg} ${backend.text}`}>
                          {backend.label}
                        </span>
                      </div>
                    </div>
                  </div>
                  <div className="flex gap-1 shrink-0">
                    {t.status === 'running' && (
                      <button
                        onClick={() => handleCancel(t.id)}
                        title="取消"
                        className="p-2 text-slate-400 hover:text-yellow-400 hover:bg-slate-700 rounded-lg transition-colors"
                      >
                        <Zap className="w-4 h-4" />
                      </button>
                    )}
                    <button
                      onClick={() => handleDelete(t.id)}
                      title="删除"
                      className="p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 rounded-lg transition-colors"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>

                <div className="space-y-2 text-sm">
                  <div className="flex justify-between items-center">
                    <span className="text-slate-400 flex items-center gap-1"><Boxes className="w-3.5 h-3.5" />模型版本</span>
                    <span className="text-slate-300 truncate ml-2" title={t.modelVersionId}>{t.modelVersionId}</span>
                  </div>

                  {done && (
                    <>
                      <div className="flex justify-between items-center">
                        <span className="text-slate-400 flex items-center gap-1"><Gauge className="w-3.5 h-3.5" />压缩比</span>
                        <span className="text-cyan-400 font-bold">{(t.compressionRatio || 0).toFixed(2)}×</span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-slate-400 flex items-center gap-1"><Gauge className="w-3.5 h-3.5" />加速比</span>
                        <span className="text-green-400 font-bold">{(t.speedup || 0).toFixed(2)}×</span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-slate-400">精度</span>
                        <span className="text-slate-300">
                          {(t.accuracy * 100).toFixed(2)}% （门禁 {(t.gateThreshold * 100).toFixed(0)}%）
                        </span>
                      </div>
                    </>
                  )}

                  {gateFailed && (
                    <div className="mt-2 text-xs text-red-400 bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2">
                      门禁未通过：{t.failReason}
                    </div>
                  )}
                  {t.status === 'failed' && !gateFailed && t.failReason && (
                    <div className="mt-2 text-xs text-red-400 bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2">
                      失败：{t.failReason}
                    </div>
                  )}
                </div>

                <div className="mt-4 w-full flex items-center justify-center gap-2 text-sm text-blue-400 border border-slate-700 rounded-lg py-2">
                  强制门禁：{done
                    ? (t.gatePass ? '放行（精度下降在阈值内）' : '拦截')
                    : '待执行完成'}
                  <ArrowRight className="w-4 h-4" />
                </div>
              </div>
            )
          })}
          {filtered.length === 0 && (
            <div className="col-span-full text-center text-slate-500 py-16">暂无加速任务，点击「创建加速任务」开始</div>
          )}
        </div>

        {showModal && (
          <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50 p-4">
            <div className="bg-slate-800 rounded-2xl p-8 w-full max-w-2xl border border-slate-700 shadow-2xl max-h-[90vh] overflow-y-auto">
              <h2 className="text-2xl font-bold text-white mb-6">创建推理加速任务</h2>
              <form onSubmit={handleCreate} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">任务名称</label>
                  <input
                    type="text"
                    value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    required
                  />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">压缩类型</label>
                    <select
                      value={form.type}
                      onChange={(e) => {
                        const type = e.target.value
                        setForm({ ...form, type, config: defaultConfig(type) })
                      }}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    >
                      <option value="quantize">量化 (Quantization)</option>
                      <option value="prune">剪枝 (Pruning)</option>
                      <option value="distill">蒸馏 (Distillation)</option>
                      <option value="convert">格式转换 (TensorRT/ONNX/OpenVINO)</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">执行后端</label>
                    <select
                      value={form.backend}
                      onChange={(e) => setForm({ ...form, backend: e.target.value })}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    >
                      <option value="pytorch">PyTorch</option>
                      <option value="onnxruntime">ONNX Runtime</option>
                      <option value="openvino">OpenVINO</option>
                    </select>
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">关联模型版本 ID</label>
                  <input
                    type="text"
                    value={form.model_version_id}
                    onChange={(e) => setForm({ ...form, model_version_id: e.target.value })}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                    placeholder="mv-xxx"
                    required
                  />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">基准精度（orig_accuracy，可选）</label>
                    <input
                      type="number" step="0.0001" min="0" max="1"
                      value={form.orig_accuracy}
                      onChange={(e) => setForm({ ...form, orig_accuracy: e.target.value })}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                      placeholder="如 0.95"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-2">门禁阈值（精度下降上限）</label>
                    <input
                      type="number" step="0.001" min="0" max="1"
                      value={form.gate_threshold}
                      onChange={(e) => setForm({ ...form, gate_threshold: e.target.value })}
                      className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500"
                      placeholder="0.01"
                    />
                    <p className="text-xs text-slate-500 mt-1">精度下降超过该值（如 1%）则门禁拦截，任务判失败</p>
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">配置 JSON</label>
                  <textarea
                    value={form.config}
                    onChange={(e) => setForm({ ...form, config: e.target.value })}
                    rows={6}
                    className="w-full bg-slate-700 border border-slate-600 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-blue-500 font-mono text-sm"
                  />
                  <p className="text-xs text-slate-500 mt-1">
                    量化：{"{method:'dynamic'|'static', bits, calibration_dataset?}"}；
                    剪枝：{"{strategy:'structured'|'unstructured', sparsity}"}；
                    蒸馏：{"{teacher_model_uri, temperature, alpha}"}；
                    转换：{"{target_format:'onnx'|'tensorrt'|'openvino'}"}
                  </p>
                </div>
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
                    className="flex-1 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white px-6 py-3 rounded-xl font-medium transition-all"
                  >
                    创建
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default InferenceAccel
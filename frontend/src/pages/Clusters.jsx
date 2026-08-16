import React, { useEffect, useState } from 'react';
import { apiFetch } from '../auth'
import { clusterStatusClass } from '../utils/status'

const API = '/api/v1';

const PROVIDERS = ['self-hosted', 'ack', 'tke', 'eks', 'gke', 'other'];

const req = async (url, options) => {
  const res = await apiFetch(url, options);
  const data = res.headers.get('content-type')?.includes('application/json') ? await res.json() : null;
  if (!res.ok) {
    throw new Error(data?.error || `请求失败 (${res.status})`);
  }
  return data;
};

export default function Clusters() {
  const [clusters, setClusters] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showModal, setShowModal] = useState(false);
  const [busyId, setBusyId] = useState(null);
  const [form, setForm] = useState({
    name: '', region: '', provider: 'self-hosted', endpoint: '',
    namespace: 'fuze-ai-paas', description: '', kube_config: ''
  });
  const [formError, setFormError] = useState(null);
  const [formBusy, setFormBusy] = useState(false);

  const fetchClusters = async () => {
    setLoading(true);
    try {
      const data = await req(`${API}/clusters`);
      setClusters(data);
      setError(null);
    } catch (e) {
      setError('加载集群失败：' + e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchClusters(); }, []);

  const handleDiscover = async (id) => {
    setBusyId(id);
    try {
      await req(`${API}/clusters/${id}/discover`, { method: 'POST' });
      await fetchClusters();
    } catch (e) {
      alert('GPU 发现失败：' + e.message);
    } finally {
      setBusyId(null);
    }
  };

  const handleTest = async (id) => {
    setBusyId(id);
    try {
      const data = await req(`${API}/clusters/${id}/test`, { method: 'POST' });
      alert(data.connected ? `连通成功，K8s 版本：${data.version}` : `连通失败：${data.error}`);
    } catch (e) {
      alert('测试失败：' + e.message);
    } finally {
      setBusyId(null);
    }
  };

  const handleDelete = async (id) => {
    if (!window.confirm('确认取消纳管该集群？其 GPU 库存将被清理。')) return;
    setBusyId(id);
    try {
      await req(`${API}/clusters/${id}`, { method: 'DELETE' });
      await fetchClusters();
    } catch (e) {
      alert('删除失败：' + e.message);
    } finally {
      setBusyId(null);
    }
  };

  const handleRegister = async (e) => {
    e.preventDefault();
    setFormBusy(true);
    setFormError(null);
    try {
      await req(`${API}/clusters`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      });
      setShowModal(false);
      setForm({ name: '', region: '', provider: 'self-hosted', endpoint: '', namespace: 'fuze-ai-paas', description: '', kube_config: '' });
      await fetchClusters();
    } catch (e) {
      setFormError('纳管失败：' + e.message);
    } finally {
      setFormBusy(false);
    }
  };

  const totalGPU = clusters.reduce((a, c) => a + (c.total_gpus || c.gpu_count || 0), 0);
  const usedGPU = clusters.reduce((a, c) => a + (c.used_gpus || 0), 0);

  return (
    <div className="app-container">
      <div className="main-content">
        <div className="content-header">
          <div>
            <h1 className="page-title">集群纳管</h1>
            <p className="page-subtitle">
              已纳管 {clusters.length} 个集群，合计 {totalGPU} 卡（已用 {usedGPU} 卡）
            </p>
          </div>
          <div className="header-actions">
            <button className="btn btn-primary" onClick={() => setShowModal(true)}>+ 纳管新集群</button>
            <button className="btn" onClick={fetchClusters}>刷新</button>
          </div>
        </div>

        {error && <div className="error-message">{error}</div>}

        {loading ? (
          <div className="loading">加载中...</div>
        ) : (
          <div className="clusters-grid">
            {clusters.map((cluster) => (
              <div key={cluster.id} className="cluster-card">
                <div className="cluster-header">
                  <div>
                    <h3 className="cluster-name">{cluster.name}</h3>
                    <span className="cluster-region">{cluster.region || cluster.provider || '—'}</span>
                  </div>
                  <span className={clusterStatusClass(cluster.status)}>
                    {cluster.status === 'healthy' ? '健康' : cluster.status === 'unhealthy' ? '异常' : '已登记'}
                  </span>
                </div>

                <div className="cluster-meta">
                  <span>节点 {cluster.node_count}</span>
                  <span>版本 {cluster.version || '—'}</span>
                  <span>{cluster.namespace}</span>
                </div>

                <div className="cluster-stats">
                  <div className="stat">
                    <div className="stat-value">{(cluster.total_gpus || cluster.gpu_count || 0)}</div>
                    <div className="stat-label">GPU 总数</div>
                  </div>
                  <div className="stat">
                    <div className="stat-value">{(cluster.used_gpus || 0)}</div>
                    <div className="stat-label">已分配</div>
                  </div>
                  <div className="stat">
                    <div className="stat-label" style={{ fontSize: 11 }}>
                      {cluster.endpoint || 'in-cluster'}
                    </div>
                  </div>
                </div>

                <div className="cluster-actions" style={{ display: 'flex', gap: 8, marginTop: 12 }}>
                  <button className="btn btn-sm" disabled={busyId === cluster.id} onClick={() => handleDiscover(cluster.id)}>
                    发现 GPU
                  </button>
                  <button className="btn btn-sm" disabled={busyId === cluster.id} onClick={() => handleTest(cluster.id)}>
                    连通测试
                  </button>
                  <button className="btn btn-sm btn-danger" disabled={busyId === cluster.id} onClick={() => handleDelete(cluster.id)}>
                    取消纳管
                  </button>
                </div>
              </div>
            ))}

            {clusters.length === 0 && !error && (
              <div className="empty-state">暂无纳管集群，点击「纳管新集群」接入 K8s。</div>
            )}
          </div>
        )}
      </div>

      {showModal && (
        <div className="modal-overlay" style={overlayStyle} onClick={() => setShowModal(false)}>
          <div className="modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
            <h2 style={{ marginTop: 0 }}>纳管新集群</h2>
            {formError && <div className="error-message">{formError}</div>}
            <form onSubmit={handleRegister}>
              <label className="form-label">集群名称 *</label>
              <input className="form-input" required value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="如 prod-gpu-cluster" />

              <div style={{ display: 'flex', gap: 12 }}>
                <div style={{ flex: 1 }}>
                  <label className="form-label">区域</label>
                  <input className="form-input" value={form.region}
                    onChange={(e) => setForm({ ...form, region: e.target.value })} placeholder="如 cn-hangzhou" />
                </div>
                <div style={{ flex: 1 }}>
                  <label className="form-label">云厂商</label>
                  <select className="form-input" value={form.provider}
                    onChange={(e) => setForm({ ...form, provider: e.target.value })}>
                    {PROVIDERS.map((p) => <option key={p} value={p}>{p}</option>)}
                  </select>
                </div>
              </div>

              <label className="form-label">APIServer 地址</label>
              <input className="form-input" value={form.endpoint}
                onChange={(e) => setForm({ ...form, endpoint: e.target.value })} placeholder="https://k8s.example.com:6443（留空则使用 in-cluster）" />

              <label className="form-label">默认命名空间</label>
              <input className="form-input" value={form.namespace}
                onChange={(e) => setForm({ ...form, namespace: e.target.value })} />

              <label className="form-label">kubeconfig（YAML）</label>
              <textarea className="form-textarea" rows={8} value={form.kube_config}
                onChange={(e) => setForm({ ...form, kube_config: e.target.value })}
                placeholder="粘贴集群的 kubeconfig 内容以纳管外部集群；留空则使用当前 in-cluster / 本地 kubeconfig" />

              <label className="form-label">描述</label>
              <input className="form-input" value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })} />

              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16 }}>
                <button type="button" className="btn" onClick={() => setShowModal(false)}>取消</button>
                <button type="submit" className="btn btn-primary" disabled={formBusy}>
                  {formBusy ? '纳管中...' : '确认纳管'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}

const overlayStyle = {
  position: 'fixed', inset: 0, background: 'rgba(10,15,30,0.6)',
  display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000,
};
const modalStyle = {
  background: '#fff', borderRadius: 12, padding: 24, width: 560, maxWidth: '92vw',
  maxHeight: '90vh', overflowY: 'auto', boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
};
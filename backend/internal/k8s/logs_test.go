package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func logTestPods() []corev1.Pod {
	pod := func(name, role string) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{"job-id": "j1", "task-role": role},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
	}
	return []corev1.Pod{pod("vj-j1-master-0", "master"), pod("vj-j1-worker-0", "worker")}
}

func names(pods []PodRef) []string {
	out := make([]string, 0, len(pods))
	for _, p := range pods {
		out = append(out, p.Name)
	}
	return out
}

func TestSelectLogPodsAggregatesAll(t *testing.T) {
	all, targets := selectLogPods(logTestPods(), LogQuery{})
	if len(all) != 2 {
		t.Fatalf("应列出 2 个副本，实际 %v", names(all))
	}
	if len(targets) != 2 {
		t.Fatalf("无过滤条件时应拉取全部副本，实际 %v", targets)
	}
	if all[0].Task != "master" || all[0].Phase != "Running" {
		t.Fatalf("副本元信息应包含角色与状态，实际 %+v", all[0])
	}
}

func TestSelectLogPodsFiltersByPod(t *testing.T) {
	all, targets := selectLogPods(logTestPods(), LogQuery{Pod: "vj-j1-worker-0"})
	if len(targets) != 1 || targets[0] != "vj-j1-worker-0" {
		t.Fatalf("应只命中指定副本，实际 %v", targets)
	}
	if len(all) != 2 {
		t.Fatalf("下钻后仍应回传全部副本选项，实际 %v", names(all))
	}
}

func TestSelectLogPodsFiltersByTask(t *testing.T) {
	_, targets := selectLogPods(logTestPods(), LogQuery{Task: "master"})
	if len(targets) != 1 || targets[0] != "vj-j1-master-0" {
		t.Fatalf("应只命中 master 角色副本，实际 %v", targets)
	}
}

func TestSelectLogPodsRequiresBothConditions(t *testing.T) {
	_, targets := selectLogPods(logTestPods(), LogQuery{Pod: "vj-j1-worker-0", Task: "master"})
	if len(targets) != 0 {
		t.Fatalf("条件矛盾时不应命中副本，实际 %v", targets)
	}
}

func TestSelectLogPodsRejectsForeignPod(t *testing.T) {
	_, targets := selectLogPods(logTestPods(), LogQuery{Pod: "vj-other-main-0"})
	if len(targets) != 0 {
		t.Fatalf("非本任务副本不应命中，实际 %v", targets)
	}
}

func TestPodTaskRoleFallsBackToVolcanoLabel(t *testing.T) {
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{"volcano.sh/task-spec": "worker"},
	}}
	if got := podTaskRole(pod); got != "worker" {
		t.Fatalf("应回退到 volcano.sh/task-spec，实际 %q", got)
	}
}
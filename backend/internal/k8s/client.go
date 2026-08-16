package k8s

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/domain/gpu"
	"fuze-ai-paas/backend/internal/domain/training"
	"fuze-ai-paas/backend/internal/k8s/chip"
	"fuze-ai-paas/backend/internal/models"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	
	volcanoJobGroup    = "batch.volcano.sh"
	volcanoJobVersion  = "v1alpha1"
	volcanoJobResource = "jobs"

	DefaultNamespace = "fuze-ai-paas"
)

type Client struct {
	dynamicClient dynamic.Interface
	clientset     kubernetes.Interface 
	namespace     string
	enabled       bool
}

type NodeGPUInfo struct {
	NodeName  string `json:"node_name"`
	GPUModel  string `json:"gpu_model"`
	GPUVendor string `json:"gpu_vendor"`
	TotalGPUs int    `json:"total_gpus"`
	UsedGPUs  int    `json:"used_gpus"`
}

func VolcanoJobGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    volcanoJobGroup,
		Version:  volcanoJobVersion,
		Resource: volcanoJobResource,
	}
}

func NewClient(namespace string) *Client {
	if namespace == "" {
		namespace = DefaultNamespace
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("[K8s] In-cluster config not found, trying kubeconfig...")
		
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Printf("[K8s] K8s client unavailable: %v — running in mock mode", err)
			return &Client{enabled: false, namespace: namespace}
		}
	}

	return NewClientFromConfig(config, namespace)
}

func NewClientFromConfig(config *rest.Config, namespace string) *Client {
	if namespace == "" {
		namespace = DefaultNamespace
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Printf("[K8s] Failed to create dynamic client: %v — running in mock mode", err)
		return &Client{enabled: false, namespace: namespace}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("[K8s] Failed to create clientset: %v — running in mock mode", err)
		return &Client{enabled: false, namespace: namespace}
	}

	log.Printf("[K8s] Client initialized, namespace=%s", namespace)
	return &Client{
		dynamicClient: dynClient,
		clientset:     clientset,
		namespace:     namespace,
		enabled:       true,
	}
}

func NewClientFromKubeConfig(kubeconfig, namespace string) (*Client, error) {
	if namespace == "" {
		namespace = DefaultNamespace
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build rest config: %w", err)
	}
	
	restConfig.Dial = safeDialer(restConfig.Dial)
	c := NewClientFromConfig(restConfig, namespace)
	return c, nil
}

func safeDialer(base DialFunc) DialFunc {
	if base == nil {
		base = (&net.Dialer{Timeout: 30 * time.Second}).DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		if ip := net.ParseIP(host); ip != nil {
			if err := assertSafeIP(ip); err != nil {
				return nil, err
			}
		} else {
			
			ips, rerr := net.LookupIP(host)
			if rerr != nil {
				return nil, fmt.Errorf("ssrf: cannot resolve %q: %w", host, rerr)
			}
			for _, ip := range ips {
				if err := assertSafeIP(ip); err != nil {
					return nil, err
				}
			}
		}
		return base(ctx, network, addr)
	}
}

func assertSafeIP(ip net.IP) error {
	if !ip.IsGlobalUnicast() {
		return fmt.Errorf("ssrf: refusing connection to non-global-unicast address %s", ip)
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("ssrf: refusing connection to loopback/link-local address %s", ip)
	}
	if ip.IsPrivate() || ip.IsUnspecified() {
		return fmt.Errorf("ssrf: refusing connection to private/unspecified address %s", ip)
	}
	return nil
}

type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func (c *Client) Enabled() bool {
	return c.enabled
}

func (c *Client) Namespace() string {
	return c.namespace
}

func SanitizeName(s string) string {
	return sanitizeName(s)
}

func (c *Client) Dynamic() dynamic.Interface {
	return c.dynamicClient
}

func (c *Client) ServerVersion(ctx context.Context) (string, error) {
	if !c.enabled || c.clientset == nil {
		return "", fmt.Errorf("k8s client not available")
	}
	v, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return v.GitVersion, nil
}

func (c *Client) DiscoverGPUInventory(ctx context.Context) ([]gpu.GPUDevice, error) {
	infos, err := c.discoverGPUNodes(ctx)
	if err != nil {
		return nil, err
	}
	return toGPUDevices(infos), nil
}

func (c *Client) discoverGPUNodes(ctx context.Context) ([]NodeGPUInfo, error) {
	if !c.enabled || c.clientset == nil {
		return nil, fmt.Errorf("k8s client not available")
	}

	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	usedByNode := make(map[string]int)
	if pods, perr := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); perr == nil {
		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.Spec.NodeName == "" {
				continue
			}
			usedByNode[pod.Spec.NodeName] += gpuRequestsOfPod(&pod.Spec)
		}
	}

	var infos []NodeGPUInfo
	for i := range nodes.Items {
		node := &nodes.Items[i]
		info := nodeGPUInfo(node)
		info.UsedGPUs = usedByNode[node.Name]
		if info.TotalGPUs > 0 {
			infos = append(infos, info)
		}
	}
	return infos, nil
}

func nodeGPUInfo(node *corev1.Node) NodeGPUInfo {
	info := NodeGPUInfo{NodeName: node.Name}

	if vendor, model, ok := chip.VendorFromNodeLabels(node.Labels); ok {
		info.GPUVendor = string(vendor)
		info.GPUModel = model
	}

	total := allocatableGPU(node.Status.Allocatable)
	info.TotalGPUs = total
	return info
}

func allocatableGPU(allocatable corev1.ResourceList) int {
	for _, name := range chip.NodeDeviceResourceKeys() {
		if q, ok := allocatable[corev1.ResourceName(name)]; ok {
			return int(q.Value())
		}
	}
	return 0
}

func gpuRequestsOfPod(spec *corev1.PodSpec) int {
	total := 0
	for _, c := range spec.Containers {
		for _, name := range chip.NodeDeviceResourceKeys() {
			if q, ok := c.Resources.Requests[corev1.ResourceName(name)]; ok {
				total += int(q.Value())
				break
			}
			if q, ok := c.Resources.Limits[corev1.ResourceName(name)]; ok {
				total += int(q.Value())
				break
			}
		}
	}
	return total
}

func (c *Client) CreateVolcanoJob(ctx context.Context, job *models.Job) (string, error) {
	if !c.enabled {
		return "", fmt.Errorf("k8s client not available")
	}

	vjName := generateVolcanoJobName(job)
	queueName := job.QueueName
	if queueName == "" {
		queueName = models.JobTypeToQueue[job.Type]
		if queueName == "" {
			queueName = "batch-queue"
		}
	}

	tasks, minAvailable, plugins := buildTasks(job)

	spec := map[string]interface{}{
		"queue":         queueName,
		"schedulerName": "volcano",
		"minAvailable":  int64(minAvailable),
		"tasks":         tasks,
	}
	if len(plugins) > 0 {
		spec["plugins"] = plugins
	}

	vj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", volcanoJobGroup, volcanoJobVersion),
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      vjName,
				"namespace": c.namespace,
				"labels": map[string]interface{}{
					"app":         "fuze-ai-paas",
					"job-id":      job.ID,
					"job-type":    string(job.Type),
					"distributed": fmt.Sprintf("%t", job.Distributed),
					"managed-by":  "fuze-scheduler",
				},
			},
			"spec": spec,
		},
	}

	gvr := VolcanoJobGVR()
	result, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).Create(ctx, vj, metav1.CreateOptions{})
	if err != nil {
		
		if apierrors.IsAlreadyExists(err) {
			log.Printf("[K8s] Volcano Job %s already exists, treating create as idempotent success", vjName)
			return vjName, nil
		}
		return "", fmt.Errorf("failed to create volcano job: %w", err)
	}

	log.Printf("[K8s] Volcano Job created: %s", result.GetName())
	return vjName, nil
}

func (c *Client) DeleteVolcanoJob(ctx context.Context, name string) error {
	if !c.enabled {
		return fmt.Errorf("k8s client not available")
	}

	gvr := VolcanoJobGVR()
	propagation := metav1.DeletePropagationForeground
	err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).Delete(
		ctx, name,
		metav1.DeleteOptions{PropagationPolicy: &propagation},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("[K8s] Volcano Job %s already absent, treating delete as idempotent success", name)
			return nil
		}
		return fmt.Errorf("failed to delete volcano job: %w", err)
	}
	log.Printf("[K8s] Volcano Job deleted: %s", name)
	return nil
}

func (c *Client) GetJobLogs(ctx context.Context, job *models.Job, query LogQuery) (JobLogs, error) {
	if !c.enabled || c.clientset == nil {
		return JobLogs{}, fmt.Errorf("k8s client not available")
	}
	tailLines := query.TailLines
	if tailLines <= 0 {
		tailLines = 100
	}

	podList, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-id=%s", job.ID),
	})
	if err != nil {
		return JobLogs{}, fmt.Errorf("failed to list pods for job %s: %w", job.ID, err)
	}

	all, targets := selectLogPods(podList.Items, query)
	result := JobLogs{Pods: all}

	if query.Pod != "" && len(targets) == 0 {
		return result, ErrPodNotFound
	}

	tail := int64(tailLines)
	var sb strings.Builder
	for _, name := range targets {
		req := c.clientset.CoreV1().Pods(c.namespace).GetLogs(name, &corev1.PodLogOptions{
			TailLines: &tail,
		})
		stream, err := req.Stream(ctx)
		if err != nil {
			log.Printf("[K8s] failed to stream logs for pod %s (job %s): %v", name, job.ID, err)
			continue
		}
		_, _ = io.Copy(&sb, stream)
		_ = stream.Close()
		sb.WriteString(fmt.Sprintf("\n--- end of logs for pod %s ---\n", name))
	}
	result.Logs = sb.String()
	return result, nil
}

func selectLogPods(pods []corev1.Pod, query LogQuery) (all []PodRef, targets []string) {
	all = make([]PodRef, 0, len(pods))
	targets = make([]string, 0, len(pods))
	for _, pod := range pods {
		role := podTaskRole(pod)
		all = append(all, PodRef{
			Name:  pod.Name,
			Task:  role,
			Phase: string(pod.Status.Phase),
		})
		if query.Pod != "" && pod.Name != query.Pod {
			continue
		}
		if query.Task != "" && role != query.Task {
			continue
		}
		targets = append(targets, pod.Name)
	}
	return all, targets
}

func podTaskRole(pod corev1.Pod) string {
	if role := pod.Labels["task-role"]; role != "" {
		return role
	}
	return pod.Labels["volcano.sh/task-spec"]
}

func (c *Client) GetVolcanoJobStatus(ctx context.Context, name string) (models.JobState, error) {
	if !c.enabled {
		return "", fmt.Errorf("k8s client not available")
	}

	gvr := VolcanoJobGVR()
	obj, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get volcano job: %w", err)
	}

	state, found, err := unstructured.NestedString(obj.Object, "status", "state", "phase")
	if err != nil || !found {
		return models.JobStatePending, nil
	}

	return models.JobState(state), nil
}

func (c *Client) SyncJobStatuses(ctx context.Context) (map[string]models.JobState, error) {
	if !c.enabled {
		return nil, fmt.Errorf("k8s client not available")
	}

	gvr := VolcanoJobGVR()
	list, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "managed-by=fuze-scheduler",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list volcano jobs: %w", err)
	}

	result := make(map[string]models.JobState)
	for _, item := range list.Items {
		name := item.GetName()
		state, _, _ := unstructured.NestedString(item.Object, "status", "state", "phase")
		result[name] = models.JobState(state)
	}

	return result, nil
}

func buildTasks(job *models.Job) ([]interface{}, int, map[string]interface{}) {
	if job.Distributed && job.Replicas > 0 {
		workerReplicas := job.Replicas
		minAvailable := 1 + workerReplicas
		if job.MinAvailable > 0 && job.MinAvailable <= minAvailable {
			minAvailable = job.MinAvailable
		}

		masterTask := buildTaskSpec(job, "master", 1)
		workerTask := buildTaskSpec(job, "worker", workerReplicas)

		plugins := map[string]interface{}{
			"env": []interface{}{},
			"svc": []interface{}{},
		}
		switch strings.ToLower(job.Framework) {
		case "pytorch", training.FrameworkPyTorchDDP, training.FrameworkDeepSpeed:
			plugins["pytorch"] = []interface{}{"--master=master", "--worker=worker", "--port=23456"}
		case training.FrameworkTensorFlow, "tf":
			plugins["tensorflow"] = []interface{}{}
		default:
			
			plugins["ssh"] = []interface{}{}
		}

		return []interface{}{masterTask, workerTask}, minAvailable, plugins
	}

	return []interface{}{buildTaskSpec(job, "main", 1)}, 1, nil
}

func buildTaskSpec(job *models.Job, taskName string, replicas int) map[string]interface{} {
	
	memoryStr := fmt.Sprintf("%dGi", job.Memory)

	limits := map[string]interface{}{
		"memory": memoryStr,
	}
	requests := map[string]interface{}{
		"memory": memoryStr,
	}

	if job.GPUs > 0 {
		spec := chip.Spec{
			Vendor:    chip.VendorOf(""), 
			GPUs:      job.GPUs,
			GPUMemory: job.GPUMemory,
			GPUCores:  job.GPUCores,
		}
		for k, v := range spec.JobResourceLimits() {
			limits[k] = v
			
			if job.GPUMemory == 0 && job.GPUCores == 0 {
				requests[k] = v
			}
		}
	}

	resources := map[string]interface{}{
		"limits":   limits,
		"requests": requests,
	}

	env := []interface{}{
		map[string]interface{}{"name": "FUZE_JOB_ID", "value": job.ID},
		map[string]interface{}{"name": "FUZE_JOB_NAME", "value": job.Name},
		map[string]interface{}{"name": "FUZE_JOB_TYPE", "value": string(job.Type)},
		map[string]interface{}{"name": "FUZE_TASK_ROLE", "value": taskName},
	}

	if job.ResumeFrom != "" {
		env = append(env, map[string]interface{}{"name": "FUZE_RESUME_FROM", "value": job.ResumeFrom})
	}
	
	if job.CodeCommit != "" {
		env = append(env, map[string]interface{}{"name": "FUZE_CODE_COMMIT", "value": job.CodeCommit})
	}
	
	if job.DataSpecJSON != "" {
		env = append(env, map[string]interface{}{"name": "FUZE_DATA_SPEC", "value": job.DataSpecJSON})
	}

	container := map[string]interface{}{
		"name":      sanitizeName(job.Name),
		"image":     job.Image,
		"command":   []interface{}{"/bin/sh", "-c"},
		"args":      []interface{}{job.Command},
		"resources": resources,
		"env":       env,
	}

	podSpec := map[string]interface{}{
		"restartPolicy": "Never",
		"containers":    []interface{}{container},
	}

	if job.MaxRuntime > 0 {
		podSpec["activeDeadlineSeconds"] = int64(job.MaxRuntime) * 60
	}

	if job.DatasetName != "" {
		mountPath := job.MountPath
		if mountPath == "" {
			mountPath = "/data"
		}
		container["volumeMounts"] = []interface{}{
			map[string]interface{}{"name": "fluid-dataset", "mountPath": mountPath},
		}
		podSpec["volumes"] = []interface{}{
			map[string]interface{}{
				"name": "fluid-dataset",
				"persistentVolumeClaim": map[string]interface{}{
					"claimName": job.DatasetName,
				},
			},
		}
	}

	return map[string]interface{}{
		"name":     taskName,
		"replicas": int64(replicas),
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"app":       "fuze-ai-paas",
					"job-id":    job.ID,
					"task-role": taskName,
				},
			},
			"spec": podSpec,
		},
	}
}

func generateVolcanoJobName(job *models.Job) string {
	return fmt.Sprintf("fuze-%s-%s", sanitizeName(string(job.Type)), job.ID)
}

func sanitizeName(name string) string {
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			sb.WriteRune(r + 32) 
		} else if r == '-' || r == '.' {
			sb.WriteRune(r)
		} else {
			
			sb.WriteRune('-')
		}
	}
	result := sb.String()
	
	result = strings.Trim(result, "-")
	if result == "" {
		return "task"
	}
	return result
}

func init() {
	
	var _ runtime.Object = &VolcanoJob{}
	var _ runtime.Object = &VolcanoJobList{}
}
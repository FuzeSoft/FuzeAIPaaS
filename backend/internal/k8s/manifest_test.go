package k8s

import (
	"testing"

	"fuze-ai-paas/backend/internal/domain/training"
	"fuze-ai-paas/backend/internal/models"
)

func asMap(t *testing.T, v interface{}, what string) map[string]interface{} {
	t.Helper()
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("%s has unexpected type %T, want map[string]interface{}", what, v)
	}
	return m
}

func asSlice(t *testing.T, v interface{}, what string) []interface{} {
	t.Helper()
	s, ok := v.([]interface{})
	if !ok {
		t.Fatalf("%s has unexpected type %T, want []interface{} (JSON 兼容类型)", what, v)
	}
	return s
}

func envOf(t *testing.T, task interface{}) map[string]string {
	t.Helper()

	container := containerOf(t, task)
	rawEnv := asSlice(t, container["env"], "container env")

	out := make(map[string]string, len(rawEnv))
	for _, raw := range rawEnv {
		e := asMap(t, raw, "env entry")
		name, _ := e["name"].(string)
		value, _ := e["value"].(string)
		out[name] = value
	}
	return out
}

func containerOf(t *testing.T, task interface{}) map[string]interface{} {
	t.Helper()

	spec := podSpecOf(t, task)
	containers := asSlice(t, spec["containers"], "containers")
	if len(containers) == 0 {
		t.Fatal("pod spec has no containers")
	}
	return asMap(t, containers[0], "container")
}

func podSpecOf(t *testing.T, task interface{}) map[string]interface{} {
	t.Helper()

	template := asMap(t, asMap(t, task, "task")["template"], "task template")
	return asMap(t, template["spec"], "pod spec")
}

func TestBuildTaskSpecInjectsResumePointer(t *testing.T) {
	job := &models.Job{
		ID:         "job-resume",
		Name:       "resume-demo",
		Type:       models.JobTypeTraining,
		Image:      "pytorch:2.3",
		Command:    "python train.py",
		Memory:     8,
		ResumeFrom: "s3://ckpt/job-resume/step-4000",
		CodeCommit: "a1b2c3d4",
	}

	env := envOf(t, buildTaskSpec(job, "main", 1))

	if got := env["FUZE_RESUME_FROM"]; got != job.ResumeFrom {
		t.Errorf("FUZE_RESUME_FROM = %q, want %q", got, job.ResumeFrom)
	}
	if got := env["FUZE_CODE_COMMIT"]; got != job.CodeCommit {
		t.Errorf("FUZE_CODE_COMMIT = %q, want %q", got, job.CodeCommit)
	}
}

func TestBuildTaskSpecOmitsEmptyOptionalEnv(t *testing.T) {
	job := &models.Job{
		ID:      "job-fresh",
		Name:    "fresh-demo",
		Type:    models.JobTypeTraining,
		Image:   "pytorch:2.3",
		Command: "python train.py",
		Memory:  8,
	}

	env := envOf(t, buildTaskSpec(job, "main", 1))

	if _, exists := env["FUZE_RESUME_FROM"]; exists {
		t.Error("FUZE_RESUME_FROM should be omitted when ResumeFrom is empty")
	}
	if _, exists := env["FUZE_CODE_COMMIT"]; exists {
		t.Error("FUZE_CODE_COMMIT should be omitted when CodeCommit is empty")
	}

	for _, key := range []string{"FUZE_JOB_ID", "FUZE_JOB_NAME", "FUZE_JOB_TYPE", "FUZE_TASK_ROLE"} {
		if _, exists := env[key]; !exists {
			t.Errorf("base env %s must always be injected", key)
		}
	}
}

func TestBuildTaskSpecAppliesMaxRuntime(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		job := &models.Job{
			ID: "job-timeout", Name: "timeout-demo", Type: models.JobTypeTraining,
			Image: "pytorch:2.3", Command: "python train.py", Memory: 8,
			MaxRuntime: 90, 
		}

		spec := podSpecOf(t, buildTaskSpec(job, "main", 1))

		got, ok := spec["activeDeadlineSeconds"].(int64)
		if !ok {
			t.Fatalf("activeDeadlineSeconds missing or wrong type: %T", spec["activeDeadlineSeconds"])
		}
		if want := int64(90 * 60); got != want {
			t.Errorf("activeDeadlineSeconds = %d, want %d", got, want)
		}
	})

	t.Run("unset means no limit", func(t *testing.T) {
		job := &models.Job{
			ID: "job-nolimit", Name: "nolimit-demo", Type: models.JobTypeTraining,
			Image: "pytorch:2.3", Command: "python train.py", Memory: 8,
		}

		spec := podSpecOf(t, buildTaskSpec(job, "main", 1))

		if _, exists := spec["activeDeadlineSeconds"]; exists {
			t.Error("activeDeadlineSeconds must be omitted when MaxRuntime is 0")
		}
	})
}

func TestBuildTasksFrameworkPluginSelection(t *testing.T) {
	cases := []struct {
		name       string
		framework  string
		wantPlugin string
	}{
		{"pytorch ddp domain constant", training.FrameworkPyTorchDDP, "pytorch"},
		{"deepspeed uses pytorch discovery", training.FrameworkDeepSpeed, "pytorch"},
		{"bare pytorch alias", "pytorch", "pytorch"},
		{"tensorflow domain constant", training.FrameworkTensorFlow, "tensorflow"},
		{"tf alias", "tf", "tensorflow"},
		{"mpi falls back to ssh", training.FrameworkMPI, "ssh"},
		{"unknown falls back to ssh", "something-else", "ssh"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := &models.Job{
				ID: "job-dist", Name: "dist-demo", Type: models.JobTypeTraining,
				Image: "pytorch:2.3", Command: "torchrun train.py", Memory: 16,
				Distributed: true, Replicas: 3, Framework: tc.framework,
			}

			_, _, plugins := buildTasks(job)

			if _, ok := plugins[tc.wantPlugin]; !ok {
				t.Errorf("framework %q: expected plugin %q, got plugins %v",
					tc.framework, tc.wantPlugin, keysOf(plugins))
			}
			
			if _, ok := plugins["svc"]; !ok {
				t.Errorf("framework %q: svc plugin must always be enabled for distributed jobs", tc.framework)
			}
		})
	}
}

func TestBuildTasksFrameworkMatchingIsCaseInsensitive(t *testing.T) {
	job := &models.Job{
		ID: "job-case", Name: "case-demo", Type: models.JobTypeTraining,
		Image: "pytorch:2.3", Command: "torchrun train.py", Memory: 16,
		Distributed: true, Replicas: 2, Framework: "PyTorch-DDP",
	}

	_, _, plugins := buildTasks(job)

	if _, ok := plugins["pytorch"]; !ok {
		t.Errorf("mixed-case framework should map to pytorch plugin, got %v", keysOf(plugins))
	}
}

func TestBuildTasksDistributedTopology(t *testing.T) {
	t.Run("default minAvailable covers all pods", func(t *testing.T) {
		job := &models.Job{
			ID: "job-topo", Name: "topo-demo", Type: models.JobTypeTraining,
			Image: "pytorch:2.3", Command: "torchrun train.py", Memory: 16,
			Distributed: true, Replicas: 3, Framework: training.FrameworkPyTorchDDP,
		}

		tasks, minAvailable, _ := buildTasks(job)

		if len(tasks) != 2 {
			t.Fatalf("expected master+worker tasks, got %d", len(tasks))
		}
		master := asMap(t, tasks[0], "tasks[0]")
		worker := asMap(t, tasks[1], "tasks[1]")
		if got := master["name"]; got != "master" {
			t.Errorf("tasks[0].name = %v, want master", got)
		}
		if got := master["replicas"]; got != int64(1) {
			t.Errorf("master replicas = %v, want 1", got)
		}
		if got := worker["name"]; got != "worker" {
			t.Errorf("tasks[1].name = %v, want worker", got)
		}
		if got := worker["replicas"]; got != int64(3) {
			t.Errorf("worker replicas = %v, want 3", got)
		}
		if minAvailable != 4 {
			t.Errorf("minAvailable = %d, want 4 (1 master + 3 workers)", minAvailable)
		}
	})

	t.Run("explicit minAvailable enables elastic scheduling", func(t *testing.T) {
		job := &models.Job{
			ID: "job-elastic", Name: "elastic-demo", Type: models.JobTypeTraining,
			Image: "pytorch:2.3", Command: "torchrun train.py", Memory: 16,
			Distributed: true, Replicas: 5, MinAvailable: 3,
			Framework: training.FrameworkPyTorchDDP,
		}

		_, minAvailable, _ := buildTasks(job)

		if minAvailable != 3 {
			t.Errorf("minAvailable = %d, want 3", minAvailable)
		}
	})

	t.Run("task role env distinguishes master from worker", func(t *testing.T) {
		job := &models.Job{
			ID: "job-role", Name: "role-demo", Type: models.JobTypeTraining,
			Image: "pytorch:2.3", Command: "torchrun train.py", Memory: 16,
			Distributed: true, Replicas: 2, Framework: training.FrameworkPyTorchDDP,
			ResumeFrom: "s3://ckpt/job-role/step-100",
		}

		tasks, _, _ := buildTasks(job)

		masterEnv := envOf(t, tasks[0])
		workerEnv := envOf(t, tasks[1])

		if masterEnv["FUZE_TASK_ROLE"] != "master" {
			t.Errorf("master FUZE_TASK_ROLE = %q", masterEnv["FUZE_TASK_ROLE"])
		}
		if workerEnv["FUZE_TASK_ROLE"] != "worker" {
			t.Errorf("worker FUZE_TASK_ROLE = %q", workerEnv["FUZE_TASK_ROLE"])
		}
		
		if workerEnv["FUZE_RESUME_FROM"] != job.ResumeFrom {
			t.Errorf("worker must also receive FUZE_RESUME_FROM, got %q", workerEnv["FUZE_RESUME_FROM"])
		}
	})
}

func TestBuildTasksSingleJobHasNoPlugins(t *testing.T) {
	job := &models.Job{
		ID: "job-single", Name: "single-demo", Type: models.JobTypeTraining,
		Image: "pytorch:2.3", Command: "python train.py", Memory: 8,
	}

	tasks, minAvailable, plugins := buildTasks(job)

	if len(tasks) != 1 {
		t.Fatalf("expected single task, got %d", len(tasks))
	}
	if got := asMap(t, tasks[0], "tasks[0]")["name"]; got != "main" {
		t.Errorf("task name = %v, want main", got)
	}
	if minAvailable != 1 {
		t.Errorf("minAvailable = %d, want 1", minAvailable)
	}
	if plugins != nil {
		t.Errorf("single job should not enable plugins, got %v", keysOf(plugins))
	}
}

func TestBuildTaskSpecGPUIsolationModes(t *testing.T) {
	t.Run("whole card sets both limits and requests", func(t *testing.T) {
		job := &models.Job{
			ID: "job-gpu", Name: "gpu-demo", Type: models.JobTypeTraining,
			Image: "pytorch:2.3", Command: "python train.py", Memory: 32, GPUs: 2,
		}

		container := containerOf(t, buildTaskSpec(job, "main", 1))
		resources := container["resources"].(map[string]interface{})
		limits := resources["limits"].(map[string]interface{})
		requests := resources["requests"].(map[string]interface{})

		if limits["nvidia.com/gpu"] != "2" {
			t.Errorf("gpu limit = %v, want 2", limits["nvidia.com/gpu"])
		}
		if requests["nvidia.com/gpu"] != "2" {
			t.Errorf("gpu request = %v, want 2", requests["nvidia.com/gpu"])
		}
	})

	t.Run("hami sets gpumem and gpucores", func(t *testing.T) {
		job := &models.Job{
			ID: "job-hami", Name: "hami-demo", Type: models.JobTypeTraining,
			Image: "pytorch:2.3", Command: "python train.py", Memory: 32,
			GPUs: 1, GPUMemory: 8192, GPUCores: 50,
		}

		container := containerOf(t, buildTaskSpec(job, "main", 1))
		limits := container["resources"].(map[string]interface{})["limits"].(map[string]interface{})

		if limits["nvidia.com/gpumem"] != "8192" {
			t.Errorf("gpumem = %v, want 8192", limits["nvidia.com/gpumem"])
		}
		if limits["nvidia.com/gpucores"] != "50" {
			t.Errorf("gpucores = %v, want 50", limits["nvidia.com/gpucores"])
		}
	})
}

func TestBuildTaskSpecDatasetMount(t *testing.T) {
	job := &models.Job{
		ID: "job-ds", Name: "ds-demo", Type: models.JobTypeTraining,
		Image: "pytorch:2.3", Command: "python train.py", Memory: 8,
		DatasetName: "imagenet",
	}

	task := buildTaskSpec(job, "main", 1)
	container := containerOf(t, task)
	spec := podSpecOf(t, task)

	mounts := asSlice(t, container["volumeMounts"], "volumeMounts")
	if len(mounts) == 0 {
		t.Fatal("expected volumeMounts")
	}
	if got := asMap(t, mounts[0], "volumeMounts[0]")["mountPath"]; got != "/data" {
		t.Errorf("default mountPath = %v, want /data", got)
	}

	volumes := asSlice(t, spec["volumes"], "volumes")
	if len(volumes) == 0 {
		t.Fatal("expected volumes")
	}
	pvc := asMap(t, asMap(t, volumes[0], "volumes[0]")["persistentVolumeClaim"], "pvc")
	if pvc["claimName"] != "imagenet" {
		t.Errorf("claimName = %v, want imagenet", pvc["claimName"])
	}
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
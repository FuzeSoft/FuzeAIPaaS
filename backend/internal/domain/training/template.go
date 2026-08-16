package training

const (
	TemplatePyTorchSingle = "pytorch-single"
	TemplatePyTorchDDP    = "pytorch-ddp"
	TemplateDeepSpeed     = "deepspeed"
	TemplateMPI           = "mpi"
)

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Spec        Spec   `json:"spec"`
}

func (t Template) NewSpec() Spec {
	s := t.Spec
	s.TemplateID = t.ID
	s.Normalize()
	return s
}

func (t Template) Apply(user Spec) Spec {
	out := user
	base := t.Spec

	if out.Image == "" {
		out.Image = base.Image
	}
	if out.Command == "" {
		out.Command = base.Command
	}
	if out.GPUs == 0 {
		out.GPUs = base.GPUs
	}
	if out.Memory == 0 {
		out.Memory = base.Memory
	}
	if out.GPUMemory == 0 {
		out.GPUMemory = base.GPUMemory
	}
	if out.GPUCores == 0 {
		out.GPUCores = base.GPUCores
	}
	if out.MaxRuntime == 0 {
		out.MaxRuntime = base.MaxRuntime
	}
	if out.Framework == "" {
		out.Framework = base.Framework
	}
	
	if !out.Distributed && base.Distributed {
		out.Distributed = true
		if out.Replicas == 0 {
			out.Replicas = base.Replicas
		}
		if out.MinAvailable == 0 {
			out.MinAvailable = base.MinAvailable
		}
	}

	out.TemplateID = t.ID
	out.Normalize()
	return out
}

var builtinTemplates = []Template{
	{
		ID:          TemplatePyTorchSingle,
		Name:        "PyTorch 单机训练",
		Description: "单节点单卡/多卡 PyTorch 训练，适合调试与中小规模微调。",
		Spec: Spec{
			Image:      "pytorch/pytorch:2.3.0-cuda12.1-cudnn8-runtime",
			Command:    "python train.py",
			GPUs:       1,
			Memory:     32,
			MaxRuntime: 24 * 60,
		},
	},
	{
		ID:          TemplatePyTorchDDP,
		Name:        "PyTorch DDP 分布式训练",
		Description: "基于 torchrun 的多节点数据并行训练，Gang 调度保证副本同起同停。",
		Spec: Spec{
			Image:        "pytorch/pytorch:2.3.0-cuda12.1-cudnn8-runtime",
			Command:      "torchrun --nnodes=$WORLD_SIZE --nproc_per_node=$GPUS_PER_NODE train.py",
			GPUs:         8,
			Memory:       128,
			Distributed:  true,
			Framework:    FrameworkPyTorchDDP,
			Replicas:     1,
			MinAvailable: 2,
			MaxRuntime:   72 * 60,
		},
	},
	{
		ID:          TemplateDeepSpeed,
		Name:        "DeepSpeed ZeRO 训练",
		Description: "面向大模型的 ZeRO 显存优化训练，适合参数量超出单卡显存的场景。",
		Spec: Spec{
			Image:        "deepspeed/deepspeed:latest",
			Command:      "deepspeed train.py --deepspeed_config ds_config.json",
			GPUs:         8,
			Memory:       256,
			Distributed:  true,
			Framework:    FrameworkDeepSpeed,
			Replicas:     3,
			MinAvailable: 4,
			MaxRuntime:   7 * 24 * 60,
		},
	},
	{
		ID:          TemplateMPI,
		Name:        "MPI 分布式训练",
		Description: "基于 MPI 的 launcher/worker 模式，适配 Horovod 等框架。",
		Spec: Spec{
			Image:        "horovod/horovod:latest",
			Command:      "mpirun -np $WORLD_SIZE python train.py",
			GPUs:         4,
			Memory:       64,
			Distributed:  true,
			Framework:    FrameworkMPI,
			Replicas:     3,
			MinAvailable: 4,
			MaxRuntime:   48 * 60,
		},
	},
}

func BuiltinTemplates() []Template {
	out := make([]Template, len(builtinTemplates))
	copy(out, builtinTemplates)
	return out
}

func FindTemplate(id string) (Template, bool) {
	for _, t := range builtinTemplates {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}
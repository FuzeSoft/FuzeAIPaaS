package k8s

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fuze-ai-paas/backend/internal/models"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFluidTestClient(ds *models.Dataset) (*Client, *dynamicfake.FakeDynamicClient) {
	scheme := runtime.NewScheme()
	gvrToKind := map[schema.GroupVersionResource]string{
		FluidDatasetGVR():           "DatasetList",
		FluidRuntimeGVR(ds.Runtime): runtimeKind(ds.Runtime) + "List",
	}
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToKind)
	return &Client{enabled: true, namespace: "fuze-ai-paas", dynamicClient: fake}, fake
}

func testDataset() *models.Dataset {
	return &models.Dataset{
		Name:       "train-corpus",
		MountPoint: "s3://bucket/corpus",
		Runtime:    models.RuntimeAlluxio,
		Replicas:   1,
	}
}

func TestCreateDatasetIsIdempotentAfterPartialFailure(t *testing.T) {
	ds := testDataset()
	c, fake := newFluidTestClient(ds)
	ctx := context.Background()

	runtimeGVR := FluidRuntimeGVR(ds.Runtime)
	failRuntime := true
	fake.PrependReactor("create", runtimeGVR.Resource,
		func(k8stesting.Action) (bool, runtime.Object, error) {
			if failRuntime {
				return true, nil, errors.New("runtime webhook rejected")
			}
			return false, nil, nil
		})
	fake.PrependReactor("delete", FluidDatasetGVR().Resource,
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("apiserver unreachable")
		})

	err := c.CreateDataset(ctx, ds)
	if err == nil {
		t.Fatal("Runtime 创建失败时必须返回错误")
	}
	
	if !strings.Contains(err.Error(), "rollback of dataset") {
		t.Errorf("回滚失败应在错误里显式暴露，实际: %v", err)
	}

	name := sanitizeName(ds.Name)
	if _, getErr := fake.Resource(FluidDatasetGVR()).Namespace(c.namespace).
		Get(ctx, name, metav1.GetOptions{}); getErr != nil {
		t.Fatalf("前置条件不成立：孤儿 Dataset 应残留，实际 %v", getErr)
	}

	failRuntime = false
	if err := c.CreateDataset(ctx, ds); err != nil {
		t.Fatalf("Dataset 已存在时重试应补建 Runtime 并成功，实际失败: %v", err)
	}
	if _, err := fake.Resource(runtimeGVR).Namespace(c.namespace).
		Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Errorf("重试后 Runtime 应已创建，实际 %v", err)
	}
}

func TestCreateDatasetFullyIdempotent(t *testing.T) {
	ds := testDataset()
	c, _ := newFluidTestClient(ds)
	ctx := context.Background()

	if err := c.CreateDataset(ctx, ds); err != nil {
		t.Fatalf("首次创建应成功: %v", err)
	}
	if err := c.CreateDataset(ctx, ds); err != nil {
		t.Fatalf("重复创建应幂等成功，实际 %v", err)
	}
}

func TestCreateDatasetRollsBackOwnDataset(t *testing.T) {
	ds := testDataset()
	c, fake := newFluidTestClient(ds)
	ctx := context.Background()

	fake.PrependReactor("create", FluidRuntimeGVR(ds.Runtime).Resource,
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("quota exceeded")
		})

	if err := c.CreateDataset(ctx, ds); err == nil {
		t.Fatal("应返回创建失败")
	}

	_, err := fake.Resource(FluidDatasetGVR()).Namespace(c.namespace).
		Get(ctx, sanitizeName(ds.Name), metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("本次新建的 Dataset 必须被回滚删除，实际 err=%v", err)
	}
}
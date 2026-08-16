package optimize

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func jobGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
}

func metav1CreateOptions() metav1.CreateOptions { return metav1.CreateOptions{} }
func metav1DeleteOptions() metav1.DeleteOptions { return metav1.DeleteOptions{} }
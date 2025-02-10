/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the \"License\");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an \"AS IS\" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k8sclient

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/pensando/device-config-manager/pkg/config_manager/globals"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/apimachinery/pkg/fields"
)

type K8sClient struct {
	sync.Mutex
	ctx       context.Context
	clientset *kubernetes.Clientset
}

func (k *K8sClient) init() error {
	k.Lock()
	defer k.Unlock()

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("k8s cluster config error %v", err)
		return err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("clientset from config failed %v", err)
		return err
	}

	k.clientset = clientset
	return nil
}

func NewClient(ctx context.Context) *K8sClient {
	return &K8sClient{
		ctx: ctx,
	}
}

func (k *K8sClient) reConnect() error {
	if k.clientset == nil {
		return k.init()
	}
	return nil
}

func GetNodeName() string {
	if os.Getenv("DS_NODE_NAME") != "" {
		return os.Getenv("DS_NODE_NAME")
	}
	return ""
}

func GetPodName() string {
	if os.Getenv("POD_NAME") != "" {
		return os.Getenv("POD_NAME")
	}
	return ""
}

func GetPodNameSpace() string {
	if os.Getenv("POD_NAMESPACE") != "" {
		return os.Getenv("POD_NAMESPACE")
	}
	return ""
}

func (k *K8sClient) GetNodeLabel(nodeName string) (map[string]string, error) {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()

	node, err := k.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		log.Printf("k8s internal node get failed %v", err)
		return make(map[string]string), err
	}
	return node.Labels, nil
}

func (k *K8sClient) GetNodeInformer() cache.SharedIndexInformer {
	k.reConnect()
	k.Lock()
	defer k.Unlock()

	// Create a shared informer factory
	factory := informers.NewSharedInformerFactory(k.clientset, 0)

	// Create a node informer
	nodeInformer := factory.Core().V1().Nodes().Informer()

	return nodeInformer
}

func (k *K8sClient) CreateEvent(evtObj *v1.Event) error {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if evtObj == nil {
		log.Printf("k8s client got empty event object, skip genreating k8s event")
		return fmt.Errorf("k8s client received empty event object")
	}

	if _, err := k.clientset.CoreV1().Events(evtObj.Namespace).Create(ctx, evtObj, metav1.CreateOptions{}); err != nil {
		log.Printf("failed to generate event %+v, err: %+v", evtObj, err)
		return err
	}

	return nil
}

func (k *K8sClient) GetDaemonSets() ([]string, bool) {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()

	daemonsetlist := make([]string, 0)
	partition_alert := false
	daemonset_count := 0
	daemonSets, err := k.clientset.AppsV1().DaemonSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})

	if err != nil {
		log.Printf("k8s internal daemonset get failed %v", err)
		return daemonsetlist, partition_alert
	}

	for _, ds := range daemonSets.Items {
		log.Printf("- %s\n", ds.Name)
		daemonsetlist = append(daemonsetlist, ds.Name)
		for key, value := range ds.Spec.Selector.MatchLabels {
			if strings.Contains(key, "daemonset-name") && strings.Contains(value, "test-deviceconfig") {
				daemonset_count = daemonset_count + 1
				break
			}
		}
	}

	if daemonset_count > globals.MAX_DAEMONSETS_ALLOWED {
		partition_alert = true
	}

	return daemonsetlist, partition_alert
}

// func CheckGpuLabel(rl v1.ResourceList) bool {
// 	s, ok := rl["amd.com/gpu"]
// 	if !ok {
// 		return false
// 	}

// 	if s.String() == "0" {
// 		return false
// 	}
// 	return true
// }

func (k *K8sClient) GetPodsToDrainOrDelete() (bool, error) {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()

	gpuPods := 0
	options := metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": GetNodeName()}).String(),
	}
	pods, err := k.clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)

	if err != nil {
		return false, err
	}

	for _, pod := range pods.Items {
		// log.Printf("Pod name: %s\n", pod.Name)
		for _, container := range pod.Spec.Containers {
			// log.Printf("Container name: %s\n", container.Name)
			if _, ok := container.Resources.Requests["amd.com/gpu"]; ok {
				// log.Printf("CONTAINER has GPU resource %v\n", container)
				// we need to check per pod level, hence break after any container
				// of the pod is requesting a gpu
				gpuPods = gpuPods + 1
				break
			}
		}
	}

	if gpuPods > globals.MAX_DAEMONSETS_ALLOWED {
		return true, nil
	}

	return false, nil
}

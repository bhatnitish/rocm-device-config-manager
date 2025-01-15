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
	"os"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
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
		fmt.Printf("k8s cluster config error %v", err)
		return err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("clientset from config failed %v", err)
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

func (k *K8sClient) GetNodelLabel(nodeName string) (map[string]string, error) {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()

	node, err := k.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("k8s internal node get failed %v", err)
		k.clientset = nil
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

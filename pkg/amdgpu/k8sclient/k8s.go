package k8sclient

import (
	"context"
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type K8sClient struct {
	sync.Mutex
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

func NewClient() *K8sClient {
	return &K8sClient{}
}

func (k *K8sClient) reConnect() error {
	if k.clientset == nil {
		return k.init()
	}
	return nil
}

func (k *K8sClient) GetNodes() (string, error) {
	k.reConnect()
	k.Lock()
	defer k.Unlock()

	// get all nodes
	nodes, err := k.clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
		return "", err
	}

	for _, node := range nodes.Items {
		fmt.Println("Node name:", node.Name)
		return node.Name, nil
	}
	return "", nil
}

func (k *K8sClient) GetNodelLabel(nodeName string) (map[string]string, error) {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
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

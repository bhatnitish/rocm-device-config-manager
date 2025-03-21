package k8e2e

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	dcmPod        *corev1.Pod
	configmapName = "test-e2e-config"
	nodePort      = 80
)

func (s *E2ESuite) addRemoveNodeLabels(nodeName string, selectedProfile string) {
	ctx := context.Background()
	err := s.k8sclient.AddNodeLabel(ctx, nodeName, "dcm.amd.com/gpu-config-profile", selectedProfile)
	if err != nil {
		log.Printf("Error adding node lbels: %s\n", err.Error())
		return
	}
	time.Sleep(45 * time.Second)
	// Allow partition to happen
	err = s.k8sclient.DeleteNodeLabel(ctx, nodeName, "dcm.amd.com/gpu-config-profile")
	if err != nil {
		log.Printf("Error removing node labels: %s\n", err.Error())
		return
	}
}

func (s *E2ESuite) getWorkerNode(ctx context.Context) string {
	labelMap := make(map[string]string)
	labelMap["feature.node.kubernetes.io/amd-gpu"] = "true"

	nodes, err := s.k8sclient.GetNodesByLabel(ctx, labelMap)
	if err != nil {
		log.Printf("Error getting nodes: %s\n", err.Error())
		return ""
	}
	worker_node := nodes[0].Name
	log.Printf("Selected Worker Node: %v", worker_node)

	return worker_node
}

func (s *E2ESuite) Test001FirstDeplymentDefaults(c *C) {
	ctx := context.Background()
	worker_node := s.getWorkerNode(ctx)
	// remove existing label for profile selection if any
	err := s.k8sclient.DeleteNodeLabel(ctx, worker_node, "dcm.amd.com/gpu-config-profile")
	if err != nil {
		log.Printf("Error removing node labels: %s\n", err.Error())
		return
	}
	log.Print("Testing helm install for exporter")
	fmt.Printf("image.repository=%v\n", s.registry)
	fmt.Printf("image.tag=%v\n", s.imageTag)
	fmt.Printf("configMap=%v\n", configmapName)
	fmt.Printf("platform=%v\n", s.platform)
	fmt.Printf("service.NodePort.nodePort=%d\n", nodePort)
	fmt.Printf("configMap=%v\n", configmapName)
	values := []string{
		fmt.Sprintf("image.repository=%v", s.registry),
		fmt.Sprintf("image.tag=%v", s.imageTag),
		fmt.Sprintf("service.NodePort.nodePort=%d", nodePort),
		fmt.Sprintf("service.type=NodePort"),
		fmt.Sprintf("configMap=%v", configmapName),
		"image.pullPolicy=Always",
	}

	err = s.k8sclient.CreateConfigMap(ctx, s.ns, configmapName)
	if err != nil {
		log.Printf("Failed to create config map %v", configmapName)
	}
	rel, err := s.helmClient.InstallChart(ctx, s.helmChart, values)
	if err != nil {
		log.Printf("failed to install charts")
		assert.Fail(c, err.Error())
		return
	}
	log.Printf("helm installed configmanager relName :%v err:%v", rel, err)
	log.Printf("sleep for 20s for pod to be ready")
	time.Sleep(20 * time.Second)
	labelMap := map[string]string{"app": "amdgpu-device-config-manager"}
	assert.Eventually(c, func() bool {
		pods, err := s.k8sclient.GetPodsByLabel(ctx, s.ns, labelMap)
		if err != nil {
			log.Printf("label get pod err %v", err)
			return false
		}
		log.Printf("pods : %+v", pods)
		if len(pods) >= 1 {
			for _, pod := range pods {
				if pod.Status.Phase == "Running" {
					dcmPod = &pod
					break
				}
			}
			return true
		}
		return false
	}, 2*time.Minute, 10*time.Second)
	assert.Eventually(c, func() bool {
		err := s.k8sclient.ValidatePod(ctx, s.ns, dcmPod.Name)
		if err != nil {
			log.Printf("label get pod err %v", err)
			return false
		}
		return true
	}, 10*time.Second, 1*time.Second)

	log.Print("Successfully deployed DCM Pod")
}

func (s *E2ESuite) Test002DCMPartitioning(c *C) {
	ctx := context.Background()

	assert.Eventually(c, func() bool {
		worker_node := s.getWorkerNode(ctx)
		s.addRemoveNodeLabels(worker_node, "default")
		labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
		if err != nil {
			return false
		}
		if len(labels) != 0 {
			gpuConfigProfileState := labels["dcm.amd.com/gpu-config-profile-state"]
			if gpuConfigProfileState != "success" {
				log.Printf("GPUConfigProfileState label reporting partition as failure")
				return false
			}
		}
		log.Printf("GPUConfigProfileState label reporting partition as success")
		return true
	}, 50*time.Second, 10*time.Second)
}

func (s *E2ESuite) Test003DCMInvalidProfiles(c *C) {
	ctx := context.Background()

	labelMap := make(map[string]string)
	labelMap["feature.node.kubernetes.io/amd-gpu"] = "true"
	assert.Eventually(c, func() bool {
		nodes, err := s.k8sclient.GetNodesByLabel(ctx, labelMap)
		worker_node := nodes[0].Name
		log.Printf("Selected Worker Node: %v", worker_node)
		if err != nil {
			log.Printf("Error getting nodes: %s\n", err.Error())
			return false
		}
		s.addRemoveNodeLabels(worker_node, "inval_prof1")
		labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
		if err != nil {
			return false
		}
		if len(labels) != 0 {
			gpuConfigProfileState := labels["dcm.amd.com/gpu-config-profile-state"]
			if gpuConfigProfileState != "failure" {
				log.Printf("Negative test case failure: GPUConfigProfileState label reporting partition as success")
				return false
			}
		}
		log.Printf("Negative test case passed: GPUConfigProfileState label reporting partition as failure")
		return true
	}, 50*time.Second, 10*time.Second)
}

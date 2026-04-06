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
	dcmPodLabels  = map[string]string{"app": "amdgpu-device-config-manager"}
)

// helmValuesDCM builds helm --set lines. Use configMap= (empty value) so upgrades clear a previously set custom name.
func (s *E2ESuite) helmValuesDCM(gpuConfigMap string) []string {
	return []string{
		fmt.Sprintf("image.repository=%v", s.registry),
		fmt.Sprintf("image.tag=%v", s.imageTag),
		fmt.Sprintf("service.NodePort.nodePort=%d", nodePort),
		"service.type=NodePort",
		fmt.Sprintf("configMap=%s", gpuConfigMap),
		"image.pullPolicy=IfNotPresent",
		"ainicHostMounts=false",
		"ainicConfigMap=",
		"createAinicConfigMap=false",
		"simEnabled=true",
	}
}

func (s *E2ESuite) findRunningDCMPod(ctx context.Context) *corev1.Pod {
	pods, err := s.k8sclient.GetPodsByLabel(ctx, s.ns, dcmPodLabels)
	if err != nil {
		return nil
	}
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodRunning {
			return &pods[i]
		}
	}
	return nil
}

// ensureDCMPodUp installs or upgrades the chart when needed. skipIfRunning: if true and a DCM pod is already Running, skip helm.
func (s *E2ESuite) ensureDCMPodUp(c *C, gpuConfigMap string, skipIfRunning bool) {
	ctx := context.Background()
	if skipIfRunning {
		if p := s.findRunningDCMPod(ctx); p != nil {
			dcmPod = p
			log.Print("DCM pod already Running; skipping helm install")
			assert.Eventually(c, func() bool {
				return s.k8sclient.ValidatePod(ctx, s.ns, dcmPod.Name) == nil
			}, 10*time.Second, 1*time.Second)
			return
		}
	}
	rel, err := s.helmClient.InstallOrUpgradeChart(ctx, s.helmChart, s.helmValuesDCM(gpuConfigMap))
	if err != nil {
		log.Printf("failed to install or upgrade charts")
		assert.Fail(c, err.Error())
		return
	}
	log.Printf("helm installed configmanager relName :%v err:%v", rel, err)
	log.Printf("sleep for 20s for pod to be ready")
	time.Sleep(20 * time.Second)
	assert.Eventually(c, func() bool {
		p := s.findRunningDCMPod(ctx)
		if p == nil {
			return false
		}
		dcmPod = p
		return true
	}, 2*time.Minute, 10*time.Second)
	if dcmPod == nil {
		assert.Fail(c, "DCM pod did not reach Running phase")
		return
	}
	assert.Eventually(c, func() bool {
		return s.k8sclient.ValidatePod(ctx, s.ns, dcmPod.Name) == nil
	}, 10*time.Second, 1*time.Second)
}

const GpuConfigProfileStateLabel = "dcm.amd.com/gpu-config-profile-state"
const GpuConfigProfileLabel = "dcm.amd.com/gpu-config-profile"

func (s *E2ESuite) addRemoveNodeLabels(nodeName string, selectedProfile string, computePartition bool) {
	ctx := context.Background()
	err := s.k8sclient.AddNodeLabel(ctx, nodeName, "dcm.amd.com/gpu-config-profile", selectedProfile)
	if err != nil {
		log.Printf("Error adding node lbels: %s\n", err.Error())
		return
	}
	if computePartition {
		time.Sleep(10 * time.Second)
	} else {
		// Memory partition requires reloading drivers which takes upto a minute
		time.Sleep(60 * time.Second)
	}

	// Allow partition to happen
	err = s.k8sclient.DeleteNodeLabel(ctx, nodeName, "dcm.amd.com/gpu-config-profile")
	if err != nil {
		log.Printf("Error removing node labels: %s\n", err.Error())
		return
	}
}

func (s *E2ESuite) getWorkerNode(c *C, ctx context.Context) string {
	labelMap := make(map[string]string)
	labelMap["feature.node.kubernetes.io/amd-gpu"] = "true"

	nodes, err := s.k8sclient.GetNodesByLabel(ctx, labelMap)
	if err != nil {
		log.Printf("Error getting nodes: %s\n", err.Error())
		assert.Fail(c, "Error getting worker node")
		return ""
	}
	if len(nodes) == 0 {
		log.Printf("No worker nodes present")
		assert.Fail(c, "Error getting worker node")
		return ""
	}
	worker_node := nodes[0].Name
	log.Printf("Selected Worker Node: %v", worker_node)

	return worker_node
}

func skipTest(c *C, reason string) {
	c.Skip(reason)
}

// skipDCMTestIfSIMRequiresGPU skips tests that need a real AMD GPU (partitioning, workload, successful partition state).
// Omit for K8s-only checks (DaemonSet, ConfigMap) and for negative profile cases that do not require hardware.
func (s *E2ESuite) skipDCMTestIfSIMRequiresGPU(c *C) {
	if s.simEnable {
		skipTest(c, "skip DCM test in SIM mode (no GPU)")
	}
}

func (s *E2ESuite) validateNodeLabels(c *C, labels map[string]string, negativeTC bool) {
	if len(labels) != 0 {
		gpuConfigProfileState := labels[GpuConfigProfileStateLabel]
		gpuConfigProfile := labels[GpuConfigProfileLabel]
		log.Printf("gpuConfigProfileState: %v\n", gpuConfigProfileState)
		log.Printf("gpuConfigProfile: %v\n", gpuConfigProfile)
		if negativeTC {
			if gpuConfigProfileState != "failure" {
				log.Printf("Negative test case failure: GPUConfigProfileState label reporting partition as success\n")
				assert.Fail(c, "Not expected (negative test case): GPUConfigProfileState -> Success")
			} else {
				log.Printf("Negative test case passed: GPUConfigProfileState label reporting partition as failure\n")
			}
		} else {
			if gpuConfigProfileState != "success" {
				log.Printf("GPUConfigProfileState label reporting partition as failure\n")
				assert.Fail(c, "GPUConfigProfileState -> Failure")
			} else {
				log.Printf("GPUConfigProfileState label reporting partition as success\n")
			}
		}
	}
}

func (s *E2ESuite) Test001DCMFirstDeplymentDefaults(c *C) {
	ctx := context.Background()
	worker_node := s.getWorkerNode(c, ctx)
	err := s.k8sclient.DeleteNodeLabel(ctx, worker_node, "dcm.amd.com/gpu-config-profile")
	if err != nil {
		log.Printf("Error removing node labels: %s\n", err.Error())
		return
	}
	log.Print("Testing helm install for exporter")
	s.ensureDCMPodUp(c, configmapName, true)
	log.Print("Successfully deployed DCM Pod")
}

func (s *E2ESuite) Test002DCMDefaultPartitioning(c *C) {
	s.skipDCMTestIfSIMRequiresGPU(c)
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: default\n")
	s.addRemoveNodeLabels(worker_node, "default", true)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, false)
}

func (s *E2ESuite) Test003DCMHeterogenousPartitioning(c *C) {
	s.skipDCMTestIfSIMRequiresGPU(c)
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: e2e_profile1\n")
	s.addRemoveNodeLabels(worker_node, "e2e_profile1", true)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, false)
}

func (s *E2ESuite) Test004DCMInvalidProfiles(c *C) {
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	labelMap := make(map[string]string)
	labelMap["feature.node.kubernetes.io/amd-gpu"] = "true"

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: e2e_profile2")
	s.addRemoveNodeLabels(worker_node, "e2e_profile2", true)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, true)
}

func (s *E2ESuite) Test005DCMInvalidComputeType(c *C) {
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	labelMap := make(map[string]string)
	labelMap["feature.node.kubernetes.io/amd-gpu"] = "true"

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: inval_prof1")
	s.addRemoveNodeLabels(worker_node, "inval_prof1", true)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, true)
}

func (s *E2ESuite) Test006DCMInvalidMemoryType(c *C) {
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	labelMap := make(map[string]string)
	labelMap["feature.node.kubernetes.io/amd-gpu"] = "true"

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: inval_prof2")
	s.addRemoveNodeLabels(worker_node, "inval_prof2", true)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, true)
}

func (s *E2ESuite) Test007DCMInvalidGPUCount(c *C) {
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	labelMap := make(map[string]string)
	labelMap["feature.node.kubernetes.io/amd-gpu"] = "true"

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: inval_prof3")
	s.addRemoveNodeLabels(worker_node, "inval_prof3", true)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, true)
}

func (s *E2ESuite) Test008DCMInvalidMemoryCombination(c *C) {
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	labelMap := make(map[string]string)
	labelMap["feature.node.kubernetes.io/amd-gpu"] = "true"

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: inval_prof4")
	s.addRemoveNodeLabels(worker_node, "inval_prof4", true)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, true)
}

func (s *E2ESuite) Test009DCMNPS4Partitioning(c *C) {
	s.skipDCMTestIfSIMRequiresGPU(c)
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: nps4\n")
	s.addRemoveNodeLabels(worker_node, "nps4", false)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, false)
}

func (s *E2ESuite) Test010DCMNPS2Partitioning(c *C) {
	s.skipDCMTestIfSIMRequiresGPU(c)
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: nps2\n")
	s.addRemoveNodeLabels(worker_node, "nps2", false)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, false)
}

func (s *E2ESuite) Test011DCMOptionalSkippedGPUs(c *C) {
	s.skipDCMTestIfSIMRequiresGPU(c)
	ctx := context.Background()
	s.ensureDCMPodUp(c, configmapName, true)

	worker_node := s.getWorkerNode(c, ctx)
	log.Printf("Adding node label to select profile: optional_filter (testing optional skipped GPUs)\n")
	s.addRemoveNodeLabels(worker_node, "optional_filter", true)
	labels, err := s.k8sclient.GetNodeLabel(ctx, worker_node)
	if err != nil {
		log.Printf("Error in getting node labels")
		assert.Fail(c, err.Error())
		return
	}
	s.validateNodeLabels(c, labels, false)
}

// Test012DCMDefaultGPUConfigMapWhenUnset verifies chart creates default-dcm-config when configMap is unset (configMap=).
// Deletes DCM pods afterward so a later test run can reinstall with the custom CM name.
func (s *E2ESuite) Test012DCMDefaultGPUConfigMapWhenUnset(c *C) {
	ctx := context.Background()
	worker_node := s.getWorkerNode(c, ctx)
	err := s.k8sclient.DeleteNodeLabel(ctx, worker_node, "dcm.amd.com/gpu-config-profile")
	if err != nil {
		log.Printf("Error removing node labels: %s\n", err.Error())
		return
	}
	log.Print("Testing helm with empty configMap (default-dcm-config)")
	s.ensureDCMPodUp(c, "", false)

	const wantName = "default-dcm-config"
	cm, err := s.k8sclient.GetConfigMap(ctx, s.ns, wantName)
	assert.NoError(c, err)
	assert.NotEmpty(c, cm.Data, "ConfigMap %s should exist with chart template data", wantName)

	err = s.k8sclient.DeletePodsByLabel(ctx, s.ns, dcmPodLabels)
	assert.NoError(c, err)
	dcmPod = nil
	log.Print("Deleted DCM pods so a follow-up test can use configMap=test-e2e-config again")
}

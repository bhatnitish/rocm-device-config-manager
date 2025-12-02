package k8e2e

import (
	"context"
	"log"
	"time"

	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	ainicDcmPod          *corev1.Pod
	ainicConfigmapName   = "ainic-e2e-tests" // Comprehensive test ConfigMap
	ainicPodDeployedOnce = false
	ainicTestWorkerNode  string
)

const AinicConfigProfileStateLabel = "dcm.amd.com/nic-config-profile-state"
const AinicConfigProfileLabel = "dcm.amd.com/nic-config-profile"
const RebootNeededLabel = "dcm.amd.com/reboot-needed"

// ensureAINICPodRunning - Ensures DCM pod is deployed and running (one-time setup)
func (s *E2ESuite) ensureAINICPodRunning(c *C) {
	if ainicPodDeployedOnce {
		log.Print("DCM pod already running, skipping deployment")
		return
	}

	ctx := context.Background()

	log.Print("Setting up AINIC E2E test environment (one-time setup)")

	// Create comprehensive test ConfigMap
	err := s.createAINICTestConfigMap(ctx)
	assert.NoError(c, err, "Failed to create test ConfigMap")

	// Deploy DCM pod
	values := []string{
		"image.repository=" + s.registry,
		"image.tag=" + s.imageTag,
		"configMap=", // Disable GPU ConfigMap
		"ainicConfigMap=" + ainicConfigmapName,
		"createAinicConfigMap=false",
		"image.pullPolicy=IfNotPresent",
	}

	rel, err := s.helmClient.InstallChart(ctx, s.helmChart, values)
	assert.NoError(c, err, "Failed to install Helm chart")
	log.Printf("Helm chart installed: %v", rel)
	time.Sleep(20 * time.Second)

	// Wait for DCM pod to be running
	labelMap := map[string]string{"app": "amdgpu-device-config-manager"}
	assert.Eventually(c, func() bool {
		pods, err := s.k8sclient.GetPodsByLabel(ctx, s.ns, labelMap)
		if err != nil || len(pods) == 0 {
			return false
		}
		for _, pod := range pods {
			if pod.Status.Phase == "Running" {
				ainicDcmPod = &pod
				log.Printf("DCM pod running: %s", pod.Name)
				return true
			}
		}
		return false
	}, 2*time.Minute, 10*time.Second)

	ainicPodDeployedOnce = true
	log.Print("AINIC test environment ready")
}

// createAINICTestConfigMap - Creates comprehensive ConfigMap with all test scenarios
func (s *E2ESuite) createAINICTestConfigMap(ctx context.Context) error {
	// Delete existing ConfigMap if present (in case Test001 created it)
	_ = s.k8sclient.DeleteConfigMap(ctx, s.ns, ainicConfigmapName)

	// Comprehensive configuration covering all test cases
	ainicConfigData := `{
    "dcqcn_profiles": {
        "p1": {"tokenBucketSize": 800000, "rateIncreaseByteCount": 431068, "cnpDscp": 46, "minRate": 1},
        "p2": {"tokenBucketSize": 900000, "rateIncreaseByteCount": 431068},
        "p3": {"tokenBucketSize": 1000000, "rateIncreaseByteCount": 431068},
        "p4": {"tokenBucketSize": 1100000, "rateIncreaseByteCount": 431068},
        "p5": {"tokenBucketSize": 1200000, "rateIncreaseByteCount": 431068},
        "p6": {"tokenBucketSize": 1300000, "rateIncreaseByteCount": 431068},
        "p7": {"tokenBucketSize": 1400000, "rateIncreaseByteCount": 431068},
        "p8": {"tokenBucketSize": 1500000, "rateIncreaseByteCount": 431068}
    },
    "port_profiles": {
        "standard": {"mtu": 1500, "admin_state": "up", "pause_type": "pfc", "rx_pause": "enable", "tx_pause": "enable"},
        "high_speed": {"mtu": 9000, "speed": "100g", "pause_type": "pfc"},
        "management": {"mtu": 1500, "speed": "10g", "pause_type": "pfc"},
        "qos_dscp_complete": {
            "mtu": 1500, "pause_type": "pfc",
            "classification_type": "dscp",
            "dscp_to_priority": [
                {"dscp": ["10"], "priority": 0},
                {"dscp": ["46"], "priority": 6},
                {"dscp": ["0-9", "11-45", "47-63"], "priority": 1}
            ]
        },
        "qos_dscp_gap": {
            "mtu": 1500, "pause_type": "pfc",
            "classification_type": "dscp",
            "dscp_to_priority": [
                {"dscp": ["10"], "priority": 0},
                {"dscp": ["46"], "priority": 6}
            ]
        },
        "qos_dscp_duplicate": {
            "mtu": 1500, "pause_type": "pfc",
            "classification_type": "dscp",
            "dscp_to_priority": [
                {"dscp": ["10"], "priority": 0},
                {"dscp": ["10"], "priority": 1}
            ]
        },
        "qos_dscp_outofrange": {
            "mtu": 1500, "pause_type": "pfc",
            "classification_type": "dscp",
            "dscp_to_priority": [
                {"dscp": ["64"], "priority": 0}
            ]
        },
        "qos_pfc_valid": {
            "mtu": 1500, "pause_type": "pfc",
            "pfc": {"priority": 0, "no_drop": "enable"}
        },
        "qos_pfc_outofrange": {
            "mtu": 1500, "pause_type": "pfc",
            "pfc": {"priority": 8, "no_drop": "enable"}
        },
        "qos_scheduling_valid": {
            "mtu": 1500, "pause_type": "pfc",
            "scheduling": {
                "priority": [0, 1, 7],
                "rate_limit": [0, 0, 10],
                "dwrr": [99, 1, 0]
            }
        },
        "qos_scheduling_mismatch": {
            "mtu": 1500, "pause_type": "pfc",
            "scheduling": {
                "priority": [0, 1],
                "rate_limit": [0, 0, 10],
                "dwrr": [99, 1, 0]
            }
        }
    },
    "nodeProfiles": {
        "port_all_test": {"nicprofiles": ["nicprof_port_all"], "reboot_type": "cold"},
        "port_specific_test": {"nicprofiles": ["nicprof_port_specific"], "reboot_type": "cold"},
        "port_mixed_test": {"nicprofiles": ["nicprof_port_mixed"], "reboot_type": "cold"},
        "port_missing_test": {"nicprofiles": ["nicprof_port_missing"], "reboot_type": "cold"},
        "port_nonexistent_test": {"nicprofiles": ["nicprof_port_nonexistent"], "reboot_type": "cold"},
        "card_missing_test": {"nicprofiles": ["nicprof_card_missing"], "reboot_type": "cold"},
        "dcqcn_provider_test": {"nicprofiles": ["nicprof_dcqcn_provider"], "reboot_type": "cold"},
        "dcqcn_workload_test": {"nicprofiles": ["nicprof_dcqcn_workload"], "reboot_type": "cold"},
        "dcqcn_provider_invalid_test": {"nicprofiles": ["nicprof_dcqcn_provider_invalid"], "reboot_type": "cold"},
        "dcqcn_workload_invalid_test": {"nicprofiles": ["nicprof_dcqcn_workload_invalid"], "reboot_type": "cold"},
        "dcqcn_missing_test": {"nicprofiles": ["nicprof_dcqcn_missing"], "reboot_type": "cold"},
        "card_reboot_test": {"nicprofiles": ["nicprof_card_reboot"], "reboot_type": "cold"},
        "qos_dscp_complete_test": {"nicprofiles": ["nicprof_qos_dscp_complete"], "reboot_type": "cold"},
        "qos_dscp_gap_test": {"nicprofiles": ["nicprof_qos_dscp_gap"], "reboot_type": "cold"},
        "qos_dscp_duplicate_test": {"nicprofiles": ["nicprof_qos_dscp_duplicate"], "reboot_type": "cold"},
        "qos_dscp_outofrange_test": {"nicprofiles": ["nicprof_qos_dscp_outofrange"], "reboot_type": "cold"},
        "qos_pfc_valid_test": {"nicprofiles": ["nicprof_qos_pfc_valid"], "reboot_type": "cold"},
        "qos_pfc_outofrange_test": {"nicprofiles": ["nicprof_qos_pfc_outofrange"], "reboot_type": "cold"},
        "qos_scheduling_valid_test": {"nicprofiles": ["nicprof_qos_scheduling_valid"], "reboot_type": "cold"},
        "qos_scheduling_mismatch_test": {"nicprofiles": ["nicprof_qos_scheduling_mismatch"], "reboot_type": "cold"},
        "multicard_overlap_test": {"nicprofiles": ["nicprof_multicard1", "nicprof_multicard2"], "reboot_type": "cold"},
        "edge_empty_nicprofiles_test": {"nicprofiles": [], "reboot_type": "cold"}
		},
    "nicProfiles": {
        "nicprof_port_all": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_port_specific": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"eth1/1": "high_speed"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_port_mixed": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"eth1/1": "high_speed", "all": "standard"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_port_missing": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"eth1/3": "high_speed"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_port_nonexistent": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "nonexistent_profile"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_card_missing": {
            "match_filters": [{"card_id": "all"}, {"pcie_address": ["0000:01:00.0"]}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_dcqcn_provider": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_dcqcn_workload": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "workload",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"]}]
        },
        "nicprof_dcqcn_provider_invalid": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1", "p2"]}]
        },
        "nicprof_dcqcn_workload_invalid": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "workload",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1", "p2"]}]
        },
        "nicprof_dcqcn_missing": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
            "dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["nonexistent_dcqcn"]}]
        },
        "nicprof_card_reboot": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "new_profile",
            "config_preference": "provider",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_qos_dscp_complete": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "qos_dscp_complete"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_qos_dscp_gap": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "qos_dscp_gap"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_qos_dscp_duplicate": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "qos_dscp_duplicate"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_qos_dscp_outofrange": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "qos_dscp_outofrange"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_qos_pfc_valid": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "qos_pfc_valid"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_qos_pfc_outofrange": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "qos_pfc_outofrange"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_qos_scheduling_valid": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "qos_scheduling_valid"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_qos_scheduling_mismatch": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "qos_scheduling_mismatch"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_multicard1": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        },
        "nicprof_multicard2": {
            "match_filters": [{"card_id": "all"}],
            "card_profile": "pf1_vf1",
            "config_preference": "provider",
            "port_profile": {"all": "standard"},
            "vf_count": 1,
			"dcqcn": [{"match_filters_dcqcn": [{"dev_id": "all"}], "profiles": ["p1"]}]
        }
    }
}`

	return s.k8sclient.CreateAINICConfigMap(ctx, s.ns, ainicConfigmapName, ainicConfigData)
}

// Helper function to apply node label and wait for configuration
func (s *E2ESuite) applyAINICProfile(nodeName string, profileName string) map[string]string {
	ctx := context.Background()

	// Apply profile label
	err := s.k8sclient.AddNodeLabel(ctx, nodeName, AinicConfigProfileLabel, profileName)
	if err != nil {
		log.Printf("Error adding AINIC node labels: %s\n", err.Error())
		return nil
	}

	// Wait for configuration to complete
	time.Sleep(10 * time.Second)

	// Get node labels
	labels, err := s.k8sclient.GetNodeLabel(ctx, nodeName)
	if err != nil {
		log.Printf("Error getting node labels: %s\n", err.Error())
		return nil
	}

	return labels
}

// Helper function to clean up node labels after test
func (s *E2ESuite) cleanupAINICLabels(nodeName string) {
	ctx := context.Background()
	s.k8sclient.DeleteNodeLabel(ctx, nodeName, AinicConfigProfileLabel)
	s.k8sclient.DeleteNodeLabel(ctx, nodeName, AinicConfigProfileStateLabel)
	s.k8sclient.DeleteNodeLabel(ctx, nodeName, RebootNeededLabel)
}

func (s *E2ESuite) getAINICWorkerNode(c *C, ctx context.Context) string {
	labelMap := make(map[string]string)
	// Assuming AINIC nodes have a specific label, adjust if different
	labelMap["feature.node.kubernetes.io/amd-nic"] = "true"

	nodes, err := s.k8sclient.GetNodesByLabel(ctx, labelMap)
	if err != nil {
		log.Printf("Error getting AINIC nodes: %s\n", err.Error())
		// Fallback: try to get any worker node with AMD GPU (DCM pod location)
		labelMap = make(map[string]string)
		labelMap["feature.node.kubernetes.io/amd-gpu"] = "true"
		nodes, err = s.k8sclient.GetNodesByLabel(ctx, labelMap)
		if err != nil {
			log.Printf("Error getting nodes: %s\n", err.Error())
			assert.Fail(c, "Error getting worker node for AINIC")
			return ""
		}
	}

	if len(nodes) == 0 {
		log.Printf("No AINIC worker nodes present")
		assert.Fail(c, "Error getting AINIC worker node")
		return ""
	}
	worker_node := nodes[0].Name
	log.Printf("Selected AINIC Worker Node: %v", worker_node)

	return worker_node
}

func (s *E2ESuite) Test001AINICBasicConfiguration(c *C) {
	ctx := context.Background()

	log.Print("Testing AINIC basic configuration")

	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Printf("image.repository=%v", s.registry)
	log.Printf("image.tag=%v", s.imageTag)
	log.Printf("ainicConfigMap=%v", ainicConfigmapName)
	log.Printf("Selected AINIC Worker Node: %v", ainicTestWorkerNode)

	labels := s.applyAINICProfile(ainicTestWorkerNode, "dcqcn_workload_test")
	assert.NotNil(c, labels, "Failed to get node labels")

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "success", state, "Expected configuration state to be 'success'")

	log.Print("Successfully completed AINIC basic configuration test")
}

func (s *E2ESuite) Test002AINICPortProfileAllKeyword(c *C) {
	ctx := context.Background()

	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("PORT-001: Testing valid 'all' port profile configuration")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "port_all_test")
	assert.NotNil(c, labels, "Failed to get node labels")

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "success", state, "Expected configuration state to be 'success'")

	_, hasRebootLabel := labels[RebootNeededLabel]
	assert.False(c, hasRebootLabel, "Reboot should not be needed for port-only changes")

	log.Print("PORT-001: Valid 'all' configuration test passed")
}

func (s *E2ESuite) Test003AINICPortProfileSpecificPorts(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("PORT-002: Testing specific port profile configuration")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "port_specific_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "success", state, "Expected configuration state to be 'success'")

	log.Print("PORT-002: Valid specific port configuration test passed")
}

func (s *E2ESuite) Test004AINICPortProfileMixedInvalid(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("PORT-003: Testing invalid mixed 'all' and specific ports (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "port_mixed_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected configuration state to be 'failure' (negative test)")

	log.Print("PORT-003: Invalid mixed configuration correctly rejected")
}

func (s *E2ESuite) Test005AINICPortProfileMissingCoverage(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("PORT-004: Testing missing port coverage (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "port_missing_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to missing port coverage")

	log.Print("PORT-004: Missing port coverage correctly detected")
}

func (s *E2ESuite) Test006AINICPortProfileNonexistent(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("PORT-005: Testing nonexistent profile reference (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "port_nonexistent_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to nonexistent profile reference")

	log.Print("PORT-005: Nonexistent profile reference correctly detected")
}

func (s *E2ESuite) Test007AINICCardCoverageMissing(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("CARD-001: Testing missing card coverage (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "card_missing_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to missing card coverage")

	log.Print("CARD-001: Missing card coverage correctly detected")
}

func (s *E2ESuite) Test008AINICDCQCNProviderMode(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("DCQCN-001: Testing valid provider mode (1 profile)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "dcqcn_provider_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "success", state, "Expected configuration state to be 'success'")

	log.Print("DCQCN-001: Valid provider mode test passed")
}

func (s *E2ESuite) Test009AINICDCQCNWorkloadMode(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("DCQCN-002: Testing valid workload mode (8 profiles)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "dcqcn_workload_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "success", state, "Expected configuration state to be 'success'")

	log.Print("DCQCN-002: Valid workload mode test passed")
}

func (s *E2ESuite) Test010AINICDCQCNProviderInvalid(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("DCQCN-003: Testing invalid profile count for provider mode (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "dcqcn_provider_invalid_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to invalid DCQCN profile count for provider mode")

	log.Print("DCQCN-003: Invalid provider mode profile count correctly detected")
}

func (s *E2ESuite) Test011AINICDCQCNWorkloadInvalid(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("DCQCN-004: Testing invalid profile count for workload mode (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "dcqcn_workload_invalid_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to invalid DCQCN profile count for workload mode")

	log.Print("DCQCN-004: Invalid workload mode profile count correctly detected")
}

func (s *E2ESuite) Test012AINICDCQCNMissingProfile(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("DCQCN-005: Testing missing DCQCN profile reference (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "dcqcn_missing_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to missing DCQCN profile reference")

	log.Print("DCQCN-005: Missing DCQCN profile reference correctly detected")
}

func (s *E2ESuite) Test013AINICQoSDSCPComplete(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("QOS-001: Testing DSCP complete coverage (all 64 values)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "qos_dscp_complete_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "success", state, "Expected configuration state to be 'success'")

	log.Print("QOS-001: DSCP complete coverage test passed")
}

func (s *E2ESuite) Test014AINICQoSDSCPGap(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("QOS-002: Testing DSCP gap coverage (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "qos_dscp_gap_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to DSCP coverage gap")

	log.Print("QOS-002: DSCP coverage gap correctly detected")
}

func (s *E2ESuite) Test015AINICQoSDSCPDuplicate(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("QOS-003: Testing DSCP duplicate mapping (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "qos_dscp_duplicate_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to duplicate DSCP mapping")

	log.Print("QOS-003: DSCP duplicate mapping correctly detected")
}

func (s *E2ESuite) Test016AINICQoSDSCPOutOfRange(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("QOS-004: Testing DSCP out of range value (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "qos_dscp_outofrange_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to DSCP value out of range")

	log.Print("QOS-004: DSCP out of range value correctly detected")
}

func (s *E2ESuite) Test017AINICQoSPFCValid(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("QOS-005: Testing PFC valid priority (0-7 range)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "qos_pfc_valid_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "success", state, "Expected configuration state to be 'success'")

	log.Print("QOS-005: PFC valid priority test passed")
}

func (s *E2ESuite) Test018AINICQoSPFCOutOfRange(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("QOS-006: Testing PFC out of range priority (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "qos_pfc_outofrange_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to PFC priority out of range")

	log.Print("QOS-006: PFC out of range priority correctly detected")
}

func (s *E2ESuite) Test019AINICQoSSchedulingValid(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("QOS-007: Testing scheduling with matching array lengths")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "qos_scheduling_valid_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "success", state, "Expected configuration state to be 'success'")

	log.Print("QOS-007: Scheduling valid array lengths test passed")
}

func (s *E2ESuite) Test020AINICQoSSchedulingMismatch(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("QOS-008: Testing scheduling array length mismatch (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "qos_scheduling_mismatch_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to scheduling array length mismatch")

	log.Print("QOS-008: Scheduling array length mismatch correctly detected")
}

func (s *E2ESuite) Test021AINICCardRebootRequired(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("CARD-002: Testing card profile change requiring reboot (new_profile)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "card_reboot_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected configuration state to be 'failure'")

	// Check if reboot label is set
	rebootRequired, exists := labels["amd.com/ainic.reboot-required"]
	log.Printf("Reboot required: %v (exists: %v)", rebootRequired, exists)
	assert.True(c, exists, "Expected reboot-required label to be set for new_profile card change")
	assert.NotEmpty(c, rebootRequired, "Expected reboot-required label to have a value")

	log.Print("CARD-002: Card profile reboot requirement test passed")
}

func (s *E2ESuite) Test022AINICEdgeEmptyNICProfiles(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("EDGE-001: Testing empty nicprofiles array (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "edge_empty_nicprofiles_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to empty nicprofiles")

	log.Print("EDGE-001: Empty nicprofiles correctly detected")
}

func (s *E2ESuite) Test023AINICMultiCardOverlap(c *C) {
	ctx := context.Background()
	s.ensureAINICPodRunning(c)

	if ainicTestWorkerNode == "" {
		ainicTestWorkerNode = s.getAINICWorkerNode(c, ctx)
	}
	defer s.cleanupAINICLabels(ainicTestWorkerNode)

	log.Print("MULTI-003: Testing overlapping card claims (negative test)")

	labels := s.applyAINICProfile(ainicTestWorkerNode, "multicard_overlap_test")
	assert.NotNil(c, labels)

	state := labels[AinicConfigProfileStateLabel]
	log.Printf("Configuration state: %s", state)
	assert.Equal(c, "failure", state, "Expected failure due to overlapping card claims")

	log.Print("MULTI-003: Overlapping card claims correctly detected")
}

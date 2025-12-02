package ainicmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	ainic_pb "github.com/ROCm/device-config-manager/gen/ainic"
	"github.com/ROCm/device-config-manager/pkg/amdgpu/k8sclient"
	"github.com/ROCm/device-config-manager/pkg/globals"
	types "github.com/ROCm/device-config-manager/pkg/interface"
	"github.com/ROCm/device-config-manager/pkg/utils"
	log_e "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const logDivider = "#####################################"

var kc *k8sclient.K8sClient = k8sclient.NewClient(context.Background())
var nodeName string = k8sclient.GetNodeName()

// Global AINIC status for event generation
var ainicStatus types.AINICStatus

func generateAINICK8sEvent(err error, event_n string) {
	generateAINICK8sEventWithContext(err, event_n, "")
}

func generateAINICK8sEventWithContext(err error, event_n string, profileContext string) {
	// Only generate events in Kubernetes mode
	if !utils.IsKubernetes() {
		return
	}

	k8sPodNamespace := k8sclient.GetPodNameSpace()
	k8sPodName := k8sclient.GetPodName()
	currTime := time.Now().UTC()

	eventType := v1.EventTypeNormal
	reason := event_n
	var message string

	if err != nil {
		eventType = v1.EventTypeWarning
	}

	msgbytes, err := json.Marshal(ainicStatus)
	if err != nil {
		log_e.Errorf("failed to marshal AINIC status message %+v err %+v", ainicStatus, err)
		return
	}
	message = string(msgbytes)

	// Add profile context if provided
	if profileContext != "" {
		message = fmt.Sprintf("Profile: %s | %s", profileContext, message)
	}

	evtObj := createAINICEventObject(event_n, k8sPodNamespace, k8sPodName, currTime, eventType, reason, message)
	kc.CreateEvent(evtObj)
}

func createAINICEventObject(event_n, k8sPodNamespace, k8sPodName string, currTime time.Time, eventType, reason, message string) *v1.Event {
	return &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: event_n,
			Namespace:    k8sPodNamespace,
		},
		FirstTimestamp: metav1.Time{
			Time: currTime,
		},
		LastTimestamp: metav1.Time{
			Time: currTime,
		},
		Count:   1,
		Type:    eventType,
		Reason:  reason,
		Message: message,
		InvolvedObject: v1.ObjectReference{
			Kind:      "Pod",
			Namespace: k8sPodNamespace,
			Name:      k8sPodName,
		},
		Source: v1.EventSource{
			Host:      nodeName,
			Component: globals.EventSourceComponentName,
		},
	}
}

func updateAINICStatusForCard(cardID, nicProfile, status, message, operation string) {
	// Only update status in Kubernetes mode
	if !utils.IsKubernetes() {
		return
	}

	// Find and update existing card status, or add new one
	for i := range ainicStatus.CardStatus {
		if ainicStatus.CardStatus[i].CardID == cardID {
			ainicStatus.CardStatus[i].Status = status
			ainicStatus.CardStatus[i].Message = message
			ainicStatus.CardStatus[i].LastOperation = operation
			return
		}
	}

	// Add new card status if not found
	ainicStatus.CardStatus = append(ainicStatus.CardStatus, types.AINICCardStatus{
		CardID:        cardID,
		NICProfile:    nicProfile,
		Status:        status,
		Message:       message,
		LastOperation: operation,
	})
}

func updateAINICStatus(finalStatus, reason string) {
	// Only update status in Kubernetes mode
	if !utils.IsKubernetes() {
		return
	}

	ainicStatus.FinalStatus = finalStatus
	ainicStatus.Reason = reason
}

func setAINICSelectedProfile(selectedProfile string) {
	// Only update status in Kubernetes mode
	if !utils.IsKubernetes() {
		return
	}

	ainicStatus.SelectedProfile = selectedProfile
}

func initializeAINICStatus() {
	// Only initialize status in Kubernetes mode
	if !utils.IsKubernetes() {
		return
	}

	ainicStatus = types.AINICStatus{
		SelectedProfile: "",
		FinalStatus:     "Configuring",
		Reason:          "AINIC configuration started",
		CardStatus:      []types.AINICCardStatus{},
	}
}

func NicctlExecute(args ...string) (string, error) {

	command := "nicctl " + strings.Join(args, " ")
	log.Printf("Executing: nicctl %s", strings.Join(args, " "))

	cmd := exec.Command("bash", "-c", command)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}
	return out.String(), nil
}

// used for testing
func DryNicctlExecute(args ...string) (string, error) {
	log.Printf("Executing: nicctl %s", strings.Join(args, " "))

	cmd := exec.Command("nicctl", args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	return out.String(), nil
}

func BashExecute(args ...string) (string, error) {

	command := "nicctl " + strings.Join(args, " ")
	log.Printf("Executing: %s", strings.Join(args, " "))

	cmd := exec.Command("bash", "-c", command)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command error: %v, stderr: %s", err, stderr.String())
	}

	return out.String(), nil
}

// used for testing
func DryBashExecute(args ...string) (string, error) {

	command := "nicctl " + strings.Join(args, " ")
	log.Printf("Executing: %s", strings.Join(args, " "))

	cmd := exec.Command("bash", "-c", command)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	return out.String(), nil
}

func GetPartitionProfile() (string, error) {

	log.Println(logDivider)
	log.Printf("AINIC profile info:\n")
	defer log.Println(logDivider)
	var selectedProfile string
	if nodeName == "" {
		err := errors.New("not a k8s deployment")
		return "", err
	}
	labels, err := kc.GetNodeLabel(nodeName)
	if err != nil {
		return "", err
	}

	if len(labels) != 0 {
		ainicConfigProfileNodeLabel := labels[globals.AinicLabelKey]

		if ainicConfigProfileNodeLabel == "" {
			log.Printf("No profile selected, please select a profile from the configmap to begin partitioning\n")
			return "", nil
		} else {
			selectedProfile = ainicConfigProfileNodeLabel
		}

		log.Printf("Selected profile name: %+v\n", selectedProfile)
	} else {
		log.Printf("No labels present on node, unusual\n")
	}
	return selectedProfile, nil
}

func StartFileWatcher(isK8s bool) {
	utils.StartFileWatcher(globals.JsonFilePathAinic, ConfigureAINICs)
}

func NodeLabelWatcher() {
	utils.NodeLabelWatcher(kc, nodeName, globals.AinicLabelKey, ConfigureAINICs)
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func getMatchingCards(filter *ainic_pb.NicMatchFilter, ids, bdfs, productNames []string) map[string]bool {
	// Start with all cards as potential matches
	allCards := make(map[string]bool)
	for _, id := range ids {
		allCards[id] = true
	}

	matches := make(map[string]bool)
	for id := range allCards {
		matches[id] = true
	}

	// Apply card_id filter (AND logic)
	if filter.CardId != "" {
		cardIdMatches := make(map[string]bool)

		if strings.EqualFold(filter.CardId, "all") {
			// "all" matches all cards
			for _, id := range ids {
				cardIdMatches[id] = true
			}
		} else {
			// Match by product name
			for i, name := range productNames {
				if strings.Contains(normalize(name), normalize(filter.CardId)) {
					cardIdMatches[ids[i]] = true
				}
			}
		}

		// Intersect with existing matches (AND logic)
		for id := range matches {
			if !cardIdMatches[id] {
				delete(matches, id)
			}
		}
	}

	// Apply pcie_address filter (AND logic)
	if len(filter.PcieAddress) > 0 {
		pcieMatches := make(map[string]bool)

		for _, bdf := range filter.PcieAddress {
			for i, actualBdf := range bdfs {
				if normalize(actualBdf) == normalize(bdf) {
					pcieMatches[ids[i]] = true
				}
			}
		}

		// Intersect with existing matches (AND logic)
		for id := range matches {
			if !pcieMatches[id] {
				delete(matches, id)
			}
		}
	}

	return matches
}

func keys(m map[string]bool) []string {
	var result []string
	for k := range m {
		result = append(result, k)
	}
	return result
}

func ValidateCardCoverage(nicProfiles []string, config *ainic_pb.Config, cardDict map[string][]string) (bool, map[string]string) {
	var ids, bdfs, productNames []string

	for id, info := range cardDict {
		if len(info) < 2 {
			log_e.Warnf("Incomplete info for card ID '%s'", id)
			continue
		}
		ids = append(ids, id)
		bdfs = append(bdfs, info[0])
		productNames = append(productNames, info[1])
	}
	if len(ids) == 0 {
		log.Printf("No cards found on node")
		return false, nil
	}

	cardClaimed := make(map[string]string) // cardID → nicProfileName

	for _, nicProf := range nicProfiles {
		nicConfig, exists := config.NicProfiles[nicProf]
		if !exists {
			log.Printf("NIC profile '%s' not found", nicProf)
			return false, cardClaimed
		}

		claimedCards := make(map[string]bool)

		for i, filter := range nicConfig.MatchFilters {
			filterMatches := getMatchingCards(filter, ids, bdfs, productNames)

			if i == 0 {
				claimedCards = filterMatches
			} else {
				// Intersect with previous
				for cardID := range claimedCards {
					if !filterMatches[cardID] {
						delete(claimedCards, cardID)
					}
				}
			}
		}

		log.Println("================================================================================")
		log.Printf("NIC profile '%s' match filters:", nicProf)
		for _, filter := range nicConfig.MatchFilters {
			log.Printf("  → card_id: '%s', pcie_address: %v", filter.CardId, filter.PcieAddress)
		}
		log.Printf("NIC profile '%s' claims cards: [%s]", nicProf, strings.Join(keys(claimedCards), ", "))
		log.Println("================================================================================")

		// Check for overlap
		for cardID := range claimedCards {
			if prev, exists := cardClaimed[cardID]; exists {
				log.Printf("Card '%s' claimed by multiple NIC profiles: '%s' and '%s'", cardID, prev, nicProf)
				return false, cardClaimed
			}
			cardClaimed[cardID] = nicProf
		}
	}

	// Check for unclaimed cards
	for _, id := range ids {
		if _, claimed := cardClaimed[id]; !claimed {
			log.Printf("Coverage gap: Card '%s' is not claimed by any NIC profile", id)
			return false, cardClaimed
		}
	}

	return true, cardClaimed
}

func GetNicProfToCardClaimMapping(cardClaimedByNic map[string]string) map[string][]string {
	nicToCards := make(map[string][]string)

	for cardID, nicProf := range cardClaimedByNic {
		nicToCards[nicProf] = append(nicToCards[nicProf], cardID)
	}

	return nicToCards
}

// ValidatePortCoverage validates that all discovered ports are covered by NIC profile port mappings
// Port names can exist on multiple cards (e.g., eth1/1 on card A and card B)
func ValidatePortCoverage(nicProfiles []string, config *ainic_pb.Config, nicToCards map[string][]string) (bool, map[string]string) {
	// Get all discovered ports per card
	cardToPorts := GetAllPortsJson()

	// Create a mapping of card+port to NIC profile for coverage tracking
	cardPortClaimed := make(map[string]string) // "cardID:portName" → nicProfileName

	// Collect all card-port combinations that need to be covered
	allCardPorts := make(map[string]bool)
	for cardID, portNames := range cardToPorts {
		for _, portName := range portNames {
			cardPortKey := fmt.Sprintf("%s:%s", cardID, portName)
			allCardPorts[cardPortKey] = true
		}
	}

	if len(allCardPorts) == 0 {
		log.Printf("No ports found on node")
		return false, nil
	}

	for _, nicProf := range nicProfiles {
		nicConfig, exists := config.NicProfiles[nicProf]
		if !exists {
			log.Printf("NIC profile '%s' not found", nicProf)
			return false, cardPortClaimed
		}

		// Get cards for this NIC profile
		cardsForThisNic := nicToCards[nicProf]
		portProfileMap := nicConfig.GetPortProfile()

		log.Println("================================================================================")
		log.Printf("NIC profile '%s' port profile mapping:", nicProf)
		for portName, profileName := range portProfileMap {
			log.Printf("  → port: '%s', profile: '%s'", portName, profileName)
		}
		log.Printf("NIC profile '%s' covers cards: %v", nicProf, cardsForThisNic)

		// Show available ports per card for this NIC profile
		for _, cardID := range cardsForThisNic {
			if ports, exists := cardToPorts[cardID]; exists {
				log.Printf("  → card '%s' has ports: [%s]", cardID, strings.Join(ports, ", "))
			}
		}
		log.Println("================================================================================")

		// Validate port profile mapping - if "all" is used, it must be the only entry
		if _, hasAll := portProfileMap["all"]; hasAll {
			if len(portProfileMap) > 1 {
				log.Printf("ERROR: NIC profile '%s' contains 'all' keyword mixed with specific port configurations. When using 'all', it must be the only entry.", nicProf)
				updateAINICStatus("Error", fmt.Sprintf("NIC profile '%s' has invalid port_profile: 'all' cannot be mixed with specific ports", nicProf))
				generateAINICK8sEvent(fmt.Errorf("invalid port_profile configuration"), globals.K8EventAINICPortCoverageValidationFailed)
				return false, cardPortClaimed
			}
		}

		// Process port profile mappings for each card in this NIC profile
		claimedCardPorts := make(map[string]bool)

		if allProfile, hasAll := portProfileMap["all"]; hasAll {
			// "all" keyword claims all ports on all cards for this NIC profile
			log.Printf("NIC profile '%s' uses 'all' keyword with profile '%s', claiming all ports on its cards", nicProf, allProfile)
			for _, cardID := range cardsForThisNic {
				if ports, exists := cardToPorts[cardID]; exists {
					for _, portName := range ports {
						cardPortKey := fmt.Sprintf("%s:%s", cardID, portName)
						claimedCardPorts[cardPortKey] = true
					}
				}
			}
		} else {
			// Specific port mappings - apply to matching ports on all cards in this NIC profile
			// First, validate that all configured ports exist on at least one card
			for configPortName := range portProfileMap {
				portExists := false
				for _, cardID := range cardsForThisNic {
					if ports, exists := cardToPorts[cardID]; exists {
						for _, availablePortName := range ports {
							if availablePortName == configPortName {
								portExists = true
								break
							}
						}
					}
					if portExists {
						break
					}
				}
				if !portExists {
					log.Printf("ERROR: NIC profile '%s' specifies port '%s' which does not exist on any of its cards", nicProf, configPortName)
					updateAINICStatus("Error", fmt.Sprintf("NIC profile '%s' specifies non-existent port '%s'", nicProf, configPortName))
					generateAINICK8sEvent(fmt.Errorf("port '%s' does not exist", configPortName), globals.K8EventAINICPortCoverageValidationFailed)
					return false, cardPortClaimed
				}
			}

			// Now claim the ports that exist
			for configPortName := range portProfileMap {
				for _, cardID := range cardsForThisNic {
					if ports, exists := cardToPorts[cardID]; exists {
						for _, availablePortName := range ports {
							if availablePortName == configPortName {
								cardPortKey := fmt.Sprintf("%s:%s", cardID, availablePortName)
								claimedCardPorts[cardPortKey] = true
								log.Printf("NIC profile '%s' claims port '%s' on card '%s'", nicProf, configPortName, cardID)
							}
						}
					}
				}
			}
		}

		// Check for overlap with other NIC profiles
		for cardPortKey := range claimedCardPorts {
			if prev, exists := cardPortClaimed[cardPortKey]; exists {
				log.Printf("Card-port '%s' claimed by multiple NIC profiles: '%s' and '%s'", cardPortKey, prev, nicProf)
				return false, cardPortClaimed
			}
			cardPortClaimed[cardPortKey] = nicProf
		}
	}

	// Check for unclaimed card-port combinations
	allCardPortsList := keys(allCardPorts)
	for _, cardPortKey := range allCardPortsList {
		if _, claimed := cardPortClaimed[cardPortKey]; !claimed {
			log.Printf("Coverage gap: Card-port '%s' is not claimed by any NIC profile", cardPortKey)
			return false, cardPortClaimed
		}
	}

	log.Printf("Port coverage validation successful. All %d card-port combinations are covered by NIC profiles", len(allCardPortsList))
	return true, cardPortClaimed
}

func isDcqcnCoverageComplete(dcqcnEntries []*ainic_pb.NicProfileDcqcnEntry, roceDeviceList []string) bool {
	covered := make(map[string]bool)
	expected := make(map[string]bool)
	extras := make(map[string]bool)

	// Normalize expected RoCE device list
	for _, roce := range roceDeviceList {
		expected[normalize(roce)] = true
	}

	// Collecting all dev_ids from all dcqcn entries
	for _, dcqcn := range dcqcnEntries {
		for _, filter := range dcqcn.GetMatchFiltersDcqcn() {
			devID := normalize(filter.GetDevId())
			if devID == "all" {
				for roce := range expected {
					covered[roce] = true
				}
			} else {
				if expected[devID] {
					covered[devID] = true
				} else {
					extras[devID] = true
					log.Printf("DCQCN filter for dev_id '%s' — not present in RoCE device list (extra entry)", devID)
				}
			}
		}
	}

	// Checking coverage status for each RoCE device
	allCovered := true
	for roce := range expected {
		if covered[roce] {
			log.Printf("RoCE device '%s' is covered by DCQCN filters", roce)
		} else {
			log.Printf("RoCE device '%s' is NOT covered by DCQCN filters", roce)
			allCovered = false
		}
	}

	return allCovered && len(extras) == 0
}

func parseDscpValues(dscpStr string) ([]int, error) {
	if strings.Contains(dscpStr, "-") {
		parts := strings.Split(dscpStr, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range format: %s", dscpStr)
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid start value in range %s: %v", dscpStr, err)
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid end value in range %s: %v", dscpStr, err)
		}
		if start > end {
			return nil, fmt.Errorf("invalid range: start %d > end %d", start, end)
		}
		values := make([]int, end-start+1)
		for i := 0; i < len(values); i++ {
			values[i] = start + i
		}
		return values, nil
	}
	val, err := strconv.Atoi(dscpStr)
	if err != nil {
		return nil, fmt.Errorf("invalid DSCP value %s: %v", dscpStr, err)
	}
	return []int{val}, nil
}

func ValidatePortProfile(portProfile *ainic_pb.PortProfile, portProfileName string) error {
	if portProfile == nil {
		return fmt.Errorf("port profile '%s' is nil", portProfileName)
	}

	// Validate DSCP to Priority mapping if present
	if len(portProfile.DscpToPriority) > 0 {
		dscpCoverage := make(map[int]int32)

		for i, mapping := range portProfile.DscpToPriority {
			if mapping.Priority > 7 {
				return fmt.Errorf("port profile '%s': invalid priority %d in mapping %d (must be 0-7)",
					portProfileName, mapping.Priority, i)
			}

			for _, dscpStr := range mapping.Dscp {
				dscpValues, err := parseDscpValues(dscpStr)
				if err != nil {
					return fmt.Errorf("port profile '%s': error parsing DSCP '%s' in mapping %d: %v",
						portProfileName, dscpStr, i, err)
				}

				for _, dscp := range dscpValues {
					if dscp < 0 || dscp > 63 {
						return fmt.Errorf("port profile '%s': DSCP value %d out of range (must be 0-63)",
							portProfileName, dscp)
					}
					if existingPriority, exists := dscpCoverage[dscp]; exists {
						return fmt.Errorf("port profile '%s': DSCP value %d is mapped to multiple priorities (%d and %d)",
							portProfileName, dscp, existingPriority, mapping.Priority)
					}
					dscpCoverage[dscp] = mapping.Priority
				}
			}
		}

		// Check for complete coverage (all 64 DSCP values 0-63)
		for dscp := 0; dscp < 64; dscp++ {
			if _, covered := dscpCoverage[dscp]; !covered {
				return fmt.Errorf("port profile '%s': DSCP value %d is not mapped to any priority",
					portProfileName, dscp)
			}
		}
	}

	// Validate PFC configuration
	if portProfile.Pfc != nil {
		if portProfile.Pfc.Priority > 7 {
			return fmt.Errorf("port profile '%s': PFC priority %d out of range (must be 0-7)",
				portProfileName, portProfile.Pfc.Priority)
		}
	}

	// Validate scheduling configuration
	if portProfile.Scheduling != nil {
		if len(portProfile.Scheduling.Priority) != len(portProfile.Scheduling.RateLimit) ||
			len(portProfile.Scheduling.Priority) != len(portProfile.Scheduling.Dwrr) {
			return fmt.Errorf("port profile '%s': scheduling arrays must have equal length (priority: %d, rate_limit: %d, dwrr: %d)",
				portProfileName, len(portProfile.Scheduling.Priority), len(portProfile.Scheduling.RateLimit), len(portProfile.Scheduling.Dwrr))
		}

		for i, priority := range portProfile.Scheduling.Priority {
			if priority > 7 {
				return fmt.Errorf("port profile '%s': scheduling priority %d at index %d out of range (must be 0-7)",
					portProfileName, priority, i)
			}
		}
	}

	log.Printf("Port profile '%s': validation passed", portProfileName)
	return nil
}

func ValidateProfile(selectedNodeProfile string, config *ainic_pb.Config, cardDict map[string][]string) (bool, map[string][]string, map[string][]string) {

	log.Printf("===============================Validating the selected node profile %s===============================", selectedNodeProfile)
	defer log.Printf("===============================End of validation the selected node profile %s===============================", selectedNodeProfile)
	// validate if selected node profile exists
	nodeProfile, ok := config.NodeProfiles[selectedNodeProfile]
	if !ok {
		log.Printf("Node profile '%s' not found in config", selectedNodeProfile)
		return false, nil, nil
	}

	// validate if nic profile/profiles of the selectedNodeProfile exists in config json
	nicProfiles := nodeProfile.Nicprofiles

	log.Println("NIC Profiles:", nicProfiles)

	for _, name := range nicProfiles {
		nicProfileConfig, ok := config.NicProfiles[name]
		if !ok {
			log_e.Warnf("NIC profile %s not found in config.NicProfiles", name)
			return false, nil, nil
		}

		portProfileMap := nicProfileConfig.GetPortProfile()

		// Validate each port profile in the map
		for portID, portProfName := range portProfileMap {
			portProf, ok := config.PortProfiles[portProfName]
			if !ok {
				log_e.Warnf("Port profile %s not found in cache for port %s", portProfName, portID)
				return false, nil, nil
			}

			// Validate port profile configuration
			if err := ValidatePortProfile(portProf, portProfName); err != nil {
				log_e.Errorf("Port profile validation failed for '%s' (port %s): %v", portProfName, portID, err)
				updateAINICStatus("Failed", fmt.Sprintf("Port profile validation failed for '%s': %v", portProfName, err))
				generateAINICK8sEvent(err, globals.K8EventAINICPortProfileValidationFailed)
				return false, nil, nil
			}
		}

		if len(nicProfileConfig.MatchFilters) == 0 {
			log.Printf("NIC profile '%s' has no match filters", name)
			return false, nil, nil
		}
		validFilter := true
		for _, filter := range nicProfileConfig.MatchFilters {
			if filter.CardId == "" && len(filter.PcieAddress) == 0 {
				log.Printf("Invalid filter in NIC profile '%s': missing card_id and pcie_address", name)
				validFilter = false
			}
		}
		if !validFilter {
			log.Printf("NIC profile '%s' missing valid \"match filter\" field", name)
			return false, nil, nil
		}

		// validate dcqcn profiles length based on config profile type
		const (
			workloadProfile = "workload"
			providerProfile = "provider"
		)

		expectedPerDevice := map[string]int{
			workloadProfile: 8,
			providerProfile: 1,
		}

		expectedCount, ok := expectedPerDevice[nicProfileConfig.GetConfigPreference()]
		if !ok {
			log.Printf("Unsupported config profile type '%s' in NIC profile '%s'. Allowed types are workload/provider modes only.", nicProfileConfig.GetConfigPreference(), name)
			return false, nil, nil
		}

		dcqcnProfiles := nicProfileConfig.GetDcqcn()
		for i, dcqcn := range dcqcnProfiles {
			for _, filter := range dcqcn.MatchFiltersDcqcn {
				deviceID := filter.DevId
				profileCount := len(dcqcn.Profiles)
				if profileCount != expectedCount {
					log.Printf("Expected profiles count for %s is %d.\nDCQCN entry %d in NIC profile '%s' has %d profiles for device '%s'.",
						nicProfileConfig.GetConfigPreference(), expectedCount, i+1, name, profileCount, deviceID)
					return false, nil, nil
				}
			}
			for _, profName := range dcqcn.Profiles {
				if _, ok := config.DcqcnProfiles[profName]; !ok {
					log.Printf("DCQCN entry %d in NIC profile '%s' has invalid profile name %s , %s not found in cache",
						i+1, name, profName, profName)
					return false, nil, nil
				}
			}
		}

	}

	valid, cardClaimedByNic := ValidateCardCoverage(nicProfiles, config, cardDict)
	if !valid {
		log.Printf("NIC profile validation failed: overlapping claims or missing coverage detected. Please ensure each card is uniquely and completely assigned to only one NIC profile.")
		updateAINICStatus("Failed", "Card coverage validation failed: overlapping claims or missing coverage")
		generateAINICK8sEvent(errors.New("card coverage validation failed"), globals.K8EventAINICCardCoverageValidationFailed)
		return false, nil, nil
	}

	// since card validations are successful, get the NIC to card mapping
	nicToCards := GetNicProfToCardClaimMapping(cardClaimedByNic)
	log.Printf("NIC profile to card mapping : %v", nicToCards)

	// Validate port coverage
	portValid, portClaimedByNic := ValidatePortCoverage(nicProfiles, config, nicToCards)
	if !portValid {
		log.Printf("Port coverage validation failed: overlapping port claims or missing port coverage detected. Please ensure each port is uniquely and completely assigned.")
		updateAINICStatus("Failed", "Port coverage validation failed: overlapping claims or missing coverage")
		generateAINICK8sEvent(errors.New("port coverage validation failed"), globals.K8EventAINICPortCoverageValidationFailed)
		return false, nil, nil
	}
	log.Printf("Port profile to port mapping : %v", portClaimedByNic)

	_, _, lifrocedevices := GetAllLifs()
	nicToLifs := GetLifsPerNicProfile(nicToCards, lifrocedevices)
	log.Printf("NIC profile to ROCE device list mapping : %v", nicToLifs)

	lifCoverage := true
	for _, name := range nicProfiles {
		nicProfile, ok := config.NicProfiles[name]
		if !ok {
			log_e.Warnf("NIC profile %s not found in config.NicProfiles", name)
			continue
		}

		roceDeviceList := nicToLifs[name]
		dcqcnProfiles := nicProfile.GetDcqcn()
		covered := isDcqcnCoverageComplete(dcqcnProfiles, roceDeviceList)
		if !covered {
			log.Printf("Dcqcn Coverage incomplete for Nic profile %v", name)
			updateAINICStatus("Failed", fmt.Sprintf("DCQCN coverage incomplete for NIC profile %s", name))
			generateAINICK8sEvent(errors.New("DCQCN coverage incomplete"), globals.K8EventAINICDCQCNCoverageValidationFailed)
			lifCoverage = false
		}
	}

	if !lifCoverage {
		return false, nil, nil
	}
	return true, nicToCards, nicToLifs
}

func GetLifsPerNicProfile(
	nicToCards map[string][]string,
	lifRoceDevices map[string][]string,
) map[string][]string {

	nicToLifs := make(map[string][]string)

	for nicProf, cardIDs := range nicToCards {
		lifSet := make(map[string]bool)

		for _, cardID := range cardIDs {
			lifs, exists := lifRoceDevices[cardID]
			if !exists {
				log.Printf("No LIFs found for card '%s' in NIC profile '%s'", cardID, nicProf)
				continue
			}

			for _, lif := range lifs {
				lifSet[lif] = true
			}
		}

		nicToLifs[nicProf] = keys(lifSet)

		log.Println("------------------------------------------------------------")
		log.Printf("NIC profile '%s' has cards: %v", nicProf, cardIDs)
		log.Printf("NIC profile '%s' has LIFs: [%s]", nicProf, strings.Join(nicToLifs[nicProf], ", "))
		log.Println("------------------------------------------------------------")
	}

	return nicToLifs
}

func ConfigureAINICs() {

	var config ainic_pb.Config
	var selectedProfile string

	// Initialize AINIC status
	initializeAINICStatus()

	file, err := ioutil.ReadFile(globals.JsonFilePathAinic)
	if err != nil {
		log_e.Errorf("Failed to read file: %v", err)
		updateAINICStatus("Failed", fmt.Sprintf("Failed to read config file: %v", err))
		generateAINICK8sEvent(err, globals.K8EventInvalidJSONInConfigMap)
		if utils.IsKubernetes() {
			err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "failure")
			if err != nil {
				log.Printf("Error adding status node label: %s\n", err.Error())
			}
		}
		return
	}
	err = protojson.Unmarshal(file, &config)
	if err != nil {
		log_e.Errorf("Failed to unmarshal JSON: %v", err)
		updateAINICStatus("Failed", fmt.Sprintf("Failed to unmarshal JSON: %v", err))
		generateAINICK8sEvent(err, globals.K8EventInvalidJSONInConfigMap)
		if utils.IsKubernetes() {
			err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "failure")
			if err != nil {
				log.Printf("Error adding status node label: %s\n", err.Error())
			}
		}
		return
	}

	if utils.IsKubernetes() {
		log.Println("Getting selected profile from node label")
		selectedProfile, err = GetPartitionProfile()
		if err != nil {
			log.Printf("Error fetching partition profile from node label %v", err)
			updateAINICStatus("Failed", fmt.Sprintf("Error fetching partition profile from node label: %v", err))
			generateAINICK8sEvent(err, globals.K8EventNonExistentProfile)
			if utils.IsKubernetes() {
				err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "failure")
				if err != nil {
					log.Printf("Error adding status node label: %s\n", err.Error())
				}
			}
			return
		}
	} else {
		log.Println("Getting selected profile from config json")
		selectedProfile = config.SelectednodeProfile
	}
	if selectedProfile == "" {
		log.Println("No profile selected, nothing to do")
		updateAINICStatus("Success", "No profile selected, nothing to do")
		if utils.IsKubernetes() {
			err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "success")
			if err != nil {
				log.Printf("Error adding status node label: %s\n", err.Error())
			}
		}
		return
	} else {
		log.Printf("Selected node profile %v", selectedProfile)
		setAINICSelectedProfile(selectedProfile)
	}

	cardDict := GetAllCardsJson()
	validProfile, nicToCards, nicToLifs := ValidateProfile(selectedProfile, &config, cardDict)
	if !validProfile {
		updateAINICStatus("Failed", "Profile validation failed")
		generateAINICK8sEvent(errors.New("profile validation failed"), globals.K8EventAINICProfileValidationFailed)
		if utils.IsKubernetes() {
			err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "failure")
			if err != nil {
				log.Printf("Error adding status node label: %s\n", err.Error())
			}
		}
		return
	}

	profDB := GetCardProfiles()
	cardProfileApplied := ApplyCardProfile(&config, nicToCards, profDB)
	if !cardProfileApplied {
		updateAINICStatus("Failed", "Card profile application failed")
		generateAINICK8sEvent(errors.New("card profile application failed"), globals.K8EventAINICCardProfileApplyFailed)
		if utils.IsKubernetes() {
			err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "failure")
			if err != nil {
				log.Printf("Error adding status node label: %s\n", err.Error())
			}
		}
		return
	}
	updateAINICStatus("Success", "Card profiles applied successfully")
	generateAINICK8sEvent(nil, globals.K8EventAINICCardProfileApplySuccess)
	// reboot check
	profDB = GetCardProfiles()
	verified := VerifyCardProfile(&config, nicToCards, profDB)
	if !verified {
		log.Printf("Please do a cold reboot to apply card profiles")
		updateAINICStatus("Failed", "Cold reboot required to apply card profiles")
		generateAINICK8sEvent(errors.New("cold reboot required"), globals.K8EventAINICRebootRequired)
		// Add reboot-needed label and set failure state
		if utils.IsKubernetes() {
			err := kc.AddNodeLabel(nodeName, globals.RebootNeededLabelKey, "true")
			if err != nil {
				log.Printf("Error adding reboot-needed node label: %s\n", err.Error())
			}
			err = kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "failure")
			if err != nil {
				log.Printf("Error adding status node label: %s\n", err.Error())
			}
		}
		return
	} else {
		// Delete the reboot-needed label when profile is verified
		if utils.IsKubernetes() {
			err := kc.DeleteNodeLabel(nodeName, globals.RebootNeededLabelKey)
			if err != nil {
				log.Printf("Error deleting reboot-needed node label: %s\n", err.Error())
			}
		}
	}
	// watch for the reboot-needed label and call a redfish API to reboot the node (we can't automate cold reboot)
	vfApplied := ApplyVFtoDevice(&config, nicToCards, cardDict)
	if !vfApplied {
		updateAINICStatus("Failed", "VF application failed")
		generateAINICK8sEvent(errors.New("VF application failed"), globals.K8EventAINICVFApplyFailed)
		if utils.IsKubernetes() {
			err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "failure")
			if err != nil {
				log.Printf("Error adding status node label: %s\n", err.Error())
			}
		}
		return
	}
	updateAINICStatus("Success", "VF applied successfully")
	generateAINICK8sEvent(nil, globals.K8EventAINICVFApplySuccess)
	cardPreferenceApplied := ApplyCardConfigPreference(&config, nicToCards)
	if !cardPreferenceApplied {
		updateAINICStatus("Failed", "Card config preference application failed")
		generateAINICK8sEvent(errors.New("card config preference application failed"), globals.K8EventAINICConfigPreferenceApplyFailed)
		if utils.IsKubernetes() {
			err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "failure")
			if err != nil {
				log.Printf("Error adding status node label: %s\n", err.Error())
			}
		}
		return
	}
	updateAINICStatus("Success", "Card config preference applied successfully")
	generateAINICK8sEvent(nil, globals.K8EventAINICConfigPreferenceApplySuccess)

	profileApplied := ParseAndApplyProfile(selectedProfile, &config, nicToCards, nicToLifs)
	if !profileApplied {
		updateAINICStatus("Failed", "Port/DCQCN profile application failed")
		if utils.IsKubernetes() {
			err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "failure")
			if err != nil {
				log.Printf("Error adding status node label: %s\n", err.Error())
			}
		}
		return
	}

	// Final success event
	updateAINICStatus("Success", "AINIC configuration completed successfully")
	generateAINICK8sEvent(nil, globals.K8EventAINICConfigurationSuccessful)
	if utils.IsKubernetes() {
		err := kc.AddNodeLabel(nodeName, globals.AinicStateLabelKey, "success")
		if err != nil {
			log.Printf("Error adding status node label: %s\n", err.Error())
		}
	}
}

func VerifyCardProfile(config *ainic_pb.Config, nicToCards map[string][]string, profDB map[string][]string) bool {
	for nicProfName, cardIDs := range nicToCards {
		log.Printf("Verifying card profile for cards in profile %v", nicProfName)
		nicProf := config.NicProfiles[nicProfName]
		cardProfileName := nicProf.CardProfile
		for _, card := range cardIDs {
			// Check if card has any profiles before accessing
			if profDB[card] == nil || len(profDB[card]) == 0 {
				log_e.Errorf("Card %s has no profiles in profDB", card)
				return false
			}

			// assuming every card has only 1 profile
			if profDB[card][0] == cardProfileName {
				log.Printf("Card %v has requested profile %v applied. Successfully verified.", card, cardProfileName)
			} else {
				log.Printf("Card %v does not have the requested profile %v applied. Card profile update unsuccessful. Current profile: %v", card, cardProfileName, profDB[card][0])
				return false
			}
		}
	}
	return true
}

func ExtractLifIDs(jsonStr string) (map[string][]string, map[string][]string, map[string][]string, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	nicsRaw, ok := parsed["nic"].([]interface{})
	if !ok {
		return nil, nil, nil, fmt.Errorf("'nic' field missing or not an array")
	}

	lifDB := make(map[string][]string)     // NIC ID → LIF IDs
	lifNameDB := make(map[string][]string) // NIC ID → Ethernet interface names
	lifRoceDB := make(map[string][]string) // NIC ID ->

	for _, nic := range nicsRaw {
		nicMap, ok := nic.(map[string]interface{})
		if !ok {
			continue
		}

		nicID, ok := nicMap["id"].(string)
		if !ok {
			continue
		}

		lifsRaw, ok := nicMap["lif"].([]interface{})
		if !ok {
			continue
		}

		var lifIDs []string
		var lifNames []string
		var roceNames []string

		for _, lif := range lifsRaw {
			lifMap, ok := lif.(map[string]interface{})
			if !ok {
				continue
			}

			if id, ok := lifMap["id"].(string); ok {
				lifIDs = append(lifIDs, id)
			}

			if ethName, ok := lifMap["ethernet_interface"].(string); ok {
				lifNames = append(lifNames, ethName)
			}

			if roceDevice, ok := lifMap["roce_interface"].(string); ok {
				roceNames = append(roceNames, roceDevice)
			}
		}

		lifDB[nicID] = lifIDs
		lifNameDB[nicID] = lifNames
		lifRoceDB[nicID] = roceNames
	}

	return lifDB, lifNameDB, lifRoceDB, nil
}

func ExtractFieldFromNics(jsonStr string, field string) ([]string, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	nicsRaw, ok := parsed["nic"].([]any)
	if !ok {
		return nil, fmt.Errorf("'nic' field missing or not an array")
	}

	var results []string
	for _, nic := range nicsRaw {
		nicMap, ok := nic.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := nicMap[field]; ok {
			if strVal, ok := value.(string); ok {
				results = append(results, strVal)
			}
		}
	}
	return results, nil
}

func GetAllCardsJson() map[string][]string {
	jsonOutput, err := NicctlExecute("show", "card", "--json", "2>/dev/null | jq .")
	if err != nil {
		log_e.Errorf("Command failed: %v", err)
		return nil
	}

	ids, _ := ExtractFieldFromNics(jsonOutput, "id")
	bdfs, _ := ExtractFieldFromNics(jsonOutput, "pcie_bdf")
	productname, _ := ExtractFieldFromNics(jsonOutput, "product_name")

	if len(ids) != len(bdfs) || len(ids) != len(productname) {
		log_e.Errorf("Mismatch in field lengths: ids=%d, bdfs=%d, names=%d", len(ids), len(bdfs), len(productname))
		return nil
	}

	cardDict := make(map[string][]string)
	for i, id := range ids {
		cardDict[id] = []string{bdfs[i], productname[i]}
	}

	return cardDict
}

func GetAllLifs() (map[string][]string, map[string][]string, map[string][]string) {
	jsonOutput, err := NicctlExecute("show", "card", "device", "--json", "2>/dev/null | jq .")
	if err != nil {
		log_e.Errorf("Command failed: %v", err)
		return nil, nil, nil
	}
	lifIDs, lifNames, lifrocedevices, _ := ExtractLifIDs(jsonOutput)
	return lifIDs, lifNames, lifrocedevices
}

func ExtractPortsFromNics(jsonStr string) (map[string][]string, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	nicsRaw, ok := parsed["nic"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("'nic' field missing or not an array")
	}

	cardToPorts := make(map[string][]string) // Card ID → Port names

	for _, nic := range nicsRaw {
		nicMap, ok := nic.(map[string]interface{})
		if !ok {
			continue
		}

		cardID, ok := nicMap["id"].(string)
		if !ok {
			continue
		}

		portsRaw, ok := nicMap["port"].([]interface{})
		if !ok {
			continue
		}

		var portNames []string
		for _, port := range portsRaw {
			portMap, ok := port.(map[string]interface{})
			if !ok {
				continue
			}

			spec, ok := portMap["spec"].(map[string]interface{})
			if !ok {
				continue
			}

			if portName, ok := spec["name"].(string); ok {
				portNames = append(portNames, portName)
			}
		}

		cardToPorts[cardID] = portNames
	}

	return cardToPorts, nil
}

// ExtractCardPortToUUIDMapping extracts card+port to UUID mapping from nicctl show port --json
func ExtractCardPortToUUIDMapping(jsonStr string) (map[string]string, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	nicsRaw, ok := parsed["nic"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("'nic' field missing or not an array")
	}

	cardPortToUUID := make(map[string]string) // "cardID:portName" → Port UUID

	for _, nic := range nicsRaw {
		nicMap, ok := nic.(map[string]interface{})
		if !ok {
			continue
		}

		cardID, ok := nicMap["id"].(string)
		if !ok {
			continue
		}

		portsRaw, ok := nicMap["port"].([]interface{})
		if !ok {
			continue
		}

		for _, port := range portsRaw {
			portMap, ok := port.(map[string]interface{})
			if !ok {
				continue
			}

			spec, ok := portMap["spec"].(map[string]interface{})
			if !ok {
				continue
			}

			portName, hasName := spec["name"].(string)
			portUUID, hasUUID := spec["id"].(string)

			if hasName && hasUUID {
				cardPortKey := fmt.Sprintf("%s:%s", cardID, portName)
				cardPortToUUID[cardPortKey] = portUUID
			}
		}
	}

	return cardPortToUUID, nil
}

func GetAllPortsJson() map[string][]string {
	jsonOutput, err := NicctlExecute("show", "port", "--json", "2>/dev/null | jq .")
	if err != nil {
		log_e.Errorf("Command failed: %v", err)
		return nil
	}

	cardToPorts, err := ExtractPortsFromNics(jsonOutput)
	if err != nil {
		log_e.Errorf("Failed to extract ports: %v", err)
		return nil
	}

	return cardToPorts
}

func GetCardPortToUUIDMapping() map[string]string {
	jsonOutput, err := NicctlExecute("show", "port", "--json", "2>/dev/null | jq .")
	if err != nil {
		log_e.Errorf("Command failed: %v", err)
		return nil
	}

	cardPortToUUID, err := ExtractCardPortToUUIDMapping(jsonOutput)
	if err != nil {
		log_e.Errorf("Failed to extract card-port to UUID mapping: %v", err)
		return nil
	}

	return cardPortToUUID
}

func GetPortsPerNicProfile(
	nicToCards map[string][]string,
	cardToPorts map[string][]string,
) map[string][]string {

	nicToPorts := make(map[string][]string)

	for nicProf, cardIDs := range nicToCards {
		portSet := make(map[string]bool)

		for _, cardID := range cardIDs {
			ports, exists := cardToPorts[cardID]
			if !exists {
				log.Printf("No ports found for card '%s' in NIC profile '%s'", cardID, nicProf)
				continue
			}

			for _, port := range ports {
				portSet[port] = true
			}
		}

		nicToPorts[nicProf] = keys(portSet)

		log.Println("------------------------------------------------------------")
		log.Printf("NIC profile '%s' has cards: %v", nicProf, cardIDs)
		log.Printf("NIC profile '%s' has ports: [%s]", nicProf, strings.Join(nicToPorts[nicProf], ", "))
		log.Println("------------------------------------------------------------")
	}

	return nicToPorts
}

// ApplyPortProfiles applies port profiles based on port-to-profile mapping
// Supports both "all" keyword (applies to all ports) and specific port targeting
// Port names in config are resolved to UUIDs for nicctl commands
// When a port name is specified, it applies to all matching ports across all cards in the NIC profile
func ApplyPortProfiles(portProfileMap map[string]string, nicToCards map[string][]string, nicProfile string, portProfiles map[string]*ainic_pb.PortProfile) bool {
	if len(portProfileMap) == 0 {
		log.Printf("No port profiles to apply")
		return true
	}

	// Get card-port to UUID mapping for specific port targeting
	cardPortToUUID := GetCardPortToUUIDMapping()
	if cardPortToUUID == nil {
		log.Printf("Failed to get card-port to UUID mapping")
		updateAINICStatus("Error", "Failed to get card-port to UUID mapping")
		generateAINICK8sEventWithContext(nil, globals.K8EventAINICPortProfileApplyFailed, "Failed to get port mapping")
		return false
	}

	// Get card-to-ports mapping
	cardToPorts := GetAllPortsJson()
	cardsForThisNic := nicToCards[nicProfile]

	// Validate port profile mapping - if "all" is used, it must be the only entry
	if _, hasAll := portProfileMap["all"]; hasAll {
		if len(portProfileMap) > 1 {
			log.Printf("ERROR: NIC profile '%s' contains 'all' keyword mixed with specific port configurations. When using 'all', it must be the only entry.", nicProfile)
			updateAINICStatus("Error", fmt.Sprintf("NIC profile '%s' has invalid port_profile: 'all' cannot be mixed with specific ports", nicProfile))
			generateAINICK8sEventWithContext(nil, globals.K8EventAINICPortProfileValidationFailed, fmt.Sprintf("Invalid port_profile in NIC profile '%s'", nicProfile))
			return false
		}
	}

	// Check if "all" keyword is used
	if allProfile, hasAll := portProfileMap["all"]; hasAll {
		// Apply the "all" profile to all ports on all cards for this NIC profile
		if portProfile, exists := portProfiles[allProfile]; exists {
			log.Printf("Applying port profile '%s' to all ports on all cards in NIC profile '%s'", allProfile, nicProfile)
			return ApplyPortProfile(portProfile, "all")
		} else {
			log.Printf("Port profile '%s' (from 'all' mapping) not found in config", allProfile)
			updateAINICStatus("Error", fmt.Sprintf("Port profile '%s' not found", allProfile))
			generateAINICK8sEventWithContext(nil, globals.K8EventAINICPortProfileValidationFailed, fmt.Sprintf("Profile: %s not found", allProfile))
			return false
		}
	}

	// Apply specific port mappings - find all matching ports across all cards
	for configPortName, profileName := range portProfileMap {
		if configPortName == "all" {
			continue // Already handled above
		}

		// Find all UUIDs for ports with this name across all cards in this NIC profile
		var matchingPortUUIDs []string
		var matchingCardPorts []string // for logging

		for _, cardID := range cardsForThisNic {
			if cardPorts, exists := cardToPorts[cardID]; exists {
				for _, portName := range cardPorts {
					if portName == configPortName {
						// Found a matching port on this card - get UUID for this card-port combination
						cardPortKey := fmt.Sprintf("%s:%s", cardID, portName)
						if portUUID, uuidExists := cardPortToUUID[cardPortKey]; uuidExists {
							matchingPortUUIDs = append(matchingPortUUIDs, portUUID)
							matchingCardPorts = append(matchingCardPorts, cardPortKey)
						} else {
							log.Printf("Warning: Port '%s' on card '%s' not found in UUID mapping", portName, cardID)
						}
					}
				}
			}
		}

		if len(matchingPortUUIDs) == 0 {
			log.Printf("Warning: No ports named '%s' found on any card in NIC profile '%s'", configPortName, nicProfile)
			continue
		}

		if portProfile, exists := portProfiles[profileName]; exists {
			log.Printf("Applying port profile '%s' to port name '%s' on cards: %v", profileName, configPortName, matchingCardPorts)

			// Apply port profile to each matching port UUID
			for i, portUUID := range matchingPortUUIDs {
				log.Printf("  → Applying to port UUID: %s (%s)", portUUID, matchingCardPorts[i])
				success := ApplyPortProfile(portProfile, portUUID)
				if !success {
					log.Printf("Failed to apply port profile '%s' to port '%s' (%s)", profileName, configPortName, matchingCardPorts[i])
					updateAINICStatus("Error", fmt.Sprintf("Failed to apply port profile '%s' to port '%s'", profileName, configPortName))
					generateAINICK8sEventWithContext(nil, globals.K8EventAINICPortProfileApplyFailed, fmt.Sprintf("Profile: %s, Port: %s", profileName, configPortName))
					return false
				}
			}
		} else {
			log.Printf("Port profile '%s' not found in config for port '%s'", profileName, configPortName)
			updateAINICStatus("Error", fmt.Sprintf("Port profile '%s' not found", profileName))
			generateAINICK8sEventWithContext(nil, globals.K8EventAINICPortProfileValidationFailed, fmt.Sprintf("Profile: %s not found", profileName))
			return false
		}
	}

	return true
}

func GetCardProfiles() map[string][]string {
	jsonStr, err := NicctlExecute("show", "card", "profile", "--json", "2>/dev/null | jq .")
	if err != nil {
		log_e.Errorf("Command failed: %v", err)
		return nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		log.Printf("failed to parse JSON: %v", err)
		return nil
	}

	nicsRaw, ok := parsed["nic"].([]interface{})
	if !ok {
		log.Printf("'nic' field missing or not an array")
		return nil
	}

	profDB := make(map[string][]string) // NIC ID → Profile names
	for _, nic := range nicsRaw {
		nicMap, ok := nic.(map[string]interface{})
		if !ok {
			continue
		}

		nicID, ok := nicMap["id"].(string)
		if !ok {
			continue
		}

		cardProfile, ok := nicMap["profile"].([]interface{})
		if !ok {
			continue
		}

		var profileNames []string
		for _, prof := range cardProfile {
			profMap, ok := prof.(map[string]interface{})
			if !ok {
				continue
			}

			if name, ok := profMap["name"].(string); ok {
				profileNames = append(profileNames, name)
			}
		}

		profDB[nicID] = profileNames
	}

	return profDB
}

func ApplyCardProfile(config *ainic_pb.Config, nicToCards map[string][]string, profDB map[string][]string) bool {
	// nicctl update card profile -p pf1_vf1  (homogenous case)
	// nicctl update card profile -p default --card <card_id> (heterogenous case)

	log.Printf("================================================ Apply Card Profile ================================================")
	for nicProfName, cardIDs := range nicToCards {
		log.Printf("Applying card profile for cards in profile %v", nicProfName)
		nicProf := config.NicProfiles[nicProfName]
		cardProfileName := nicProf.CardProfile
		for _, card := range cardIDs {
			// Check if card has any profiles before accessing
			if profDB[card] == nil || len(profDB[card]) == 0 {
				log_e.Errorf("Card %s has no profiles in profDB", card)
				updateAINICStatusForCard(card, nicProfName, "Failed", "No profiles found for card", "ApplyCardProfile")
				generateAINICK8sEventWithContext(errors.New("no profiles found for card"), globals.K8EventAINICNicctlCommandFailed, fmt.Sprintf("NIC Profile: %s, Card: %s", nicProfName, card))
				return false
			}

			// assuming every card has only 1 profile
			if profDB[card][0] == cardProfileName {
				log.Printf("Card %v already has requested profile %v applied. Hence skipping update.", card, cardProfileName)
				updateAINICStatusForCard(card, nicProfName, "Success", "Card profile already applied", "ApplyCardProfile")
				continue
			}
			out, err := NicctlExecute("update", "card", "profile", "-p", cardProfileName, "--card ", card)
			if err != nil {
				log_e.Errorf("Failed to update card profile for card %s: %v", card, err)
				updateAINICStatusForCard(card, nicProfName, "Failed", fmt.Sprintf("Failed to update card profile: %v", err), "ApplyCardProfile")
				generateAINICK8sEvent(err, globals.K8EventAINICNicctlCommandFailed)
				return false
			} else {
				log.Printf("%v", out)
				updateAINICStatusForCard(card, nicProfName, "Success", "Card profile applied successfully", "ApplyCardProfile")
			}
		}
	}
	log.Printf("================================================ End of Apply Card Profile ================================================")
	return true
}

func ApplyCardConfigPreference(config *ainic_pb.Config, nicToCards map[string][]string) bool {
	// nicctl update card config-preference <--provider | --workload> [ --card <uuid> | --bdf <bdf> ]

	log.Printf("================================================ Apply card config preference to cards ================================================")
	for nicProfName, cardIDs := range nicToCards {
		log.Printf("Applying card config for cards in profile %v", nicProfName)
		nicProf := config.NicProfiles[nicProfName]
		cardConfigProfile := "--" + nicProf.GetConfigPreference()
		for _, card := range cardIDs {
			out, err := NicctlExecute("update", "card", "config-preference", cardConfigProfile, "--card ", card)
			if err != nil {
				log_e.Errorf("Failed to update card config-preference for card %s: %v", card, err)
				updateAINICStatusForCard(card, nicProfName, "Failed", fmt.Sprintf("Failed to update config preference: %v", err), "ApplyCardConfigPreference")
				generateAINICK8sEvent(err, globals.K8EventAINICNicctlCommandFailed)
				return false
			} else {
				log.Printf("%v", out)
				updateAINICStatusForCard(card, nicProfName, "Success", "Config preference applied successfully", "ApplyCardConfigPreference")
			}
		}
	}
	log.Printf("================================================ End of Apply card config preference to cards ================================================")
	return true

}

func ApplyVFtoDevice(config *ainic_pb.Config, nicToCards map[string][]string, cardDict map[string][]string) bool {

	// echo 1 > /sys/bus/pci/devices/0000\:04\:00.0/sriov_numvfs. (OR)
	// echo 1 > /sys/class/net/enp132s0/device/sriov_numvfs

	log.Printf("================================================ Apply VF to PF Device ================================================")
	for nicProfName, cardIDs := range nicToCards {
		log.Printf("Applying VF for cards in profile %v", nicProfName)
		nicProf := config.NicProfiles[nicProfName]
		vfCount := nicProf.VfCount
		for _, card := range cardIDs {
			cardBDF := cardDict[card][0]
			path := "/sys/bus/pci/devices/" + cardBDF + "/sriov_numvfs"
			vfStr := strconv.Itoa(int(vfCount))
			out, err := BashExecute("echo", vfStr, ">", path)
			if err != nil {
				log_e.Errorf("Failed to apply VF for card BDF %s: %v", cardBDF, err)
				updateAINICStatusForCard(card, nicProfName, "Failed", fmt.Sprintf("Failed to apply VF: %v", err), "ApplyVFtoDevice")
				generateAINICK8sEvent(err, globals.K8EventAINICNicctlCommandFailed)
				return false
			} else {
				log.Printf("%v", out)
				updateAINICStatusForCard(card, nicProfName, "Success", "VF applied successfully", "ApplyVFtoDevice")
			}
		}
	}
	log.Printf("================================================ End of Apply VF to PF Device ================================================")
	return true
}

func ApplyPortProfile(portProfile *ainic_pb.PortProfile, portIdentifier string) bool {

	// nicctl update port --all --pause-type  pfc --rx-pause enable --tx-pause enable  (for all ports)
	// nicctl update port -p <port_uuid> --pause-type  pfc --rx-pause enable --tx-pause enable  (for specific port)
	// nicctl update port --all --mtu 1500  (for all ports)
	// nicctl update port -p <port_uuid> --mtu 1500  (for specific port)
	// nicctl update qos --classification-type DSCP  (global setting)
	// nicctl update qos -p <port_uuid> dscp-to-priority --dscp 10 --priority 0  (for specific port)
	// nicctl update qos -p <port_uuid> pfc --priority 0 --no-drop enable  (for specific port)
	// nicctl update qos -p <port_uuid> scheduling --priority 0,1,6 --rate-limit 0,0,10 --dwrr 99,1,0  (for specific port)

	// Determine port argument: --all for all ports, -p <port_uuid> for specific port
	var portArg []string
	if portIdentifier == "all" {
		portArg = []string{"--all"}
		log.Printf("Applying port profile to all ports")
	} else {
		portArg = []string{"-p", portIdentifier}
		log.Printf("Applying port profile to specific port UUID: %s", portIdentifier)
	}

	//  MTU
	if portProfile.Mtu > 0 {
		args := append([]string{"update", "port"}, portArg...)
		args = append(args, "--mtu", fmt.Sprintf("%d", portProfile.Mtu))
		out, err := NicctlExecute(args...)
		if err != nil {
			log_e.Errorf("Failed to update MTU for port %s: %v", portIdentifier, err)
			updateAINICStatus("Failed", fmt.Sprintf("Failed to update MTU for port %s: %v", portIdentifier, err))
			generateAINICK8sEvent(err, globals.K8EventAINICPortProfileApplyFailed)
			return false
		} else {
			log.Printf("%v", out)
		}
	}

	if portProfile.PauseType != "" {
		args := append([]string{"update", "port"}, portArg...)
		args = append(args,
			"--pause-type", portProfile.PauseType,
			"--rx-pause", portProfile.RxPause,
			"--tx-pause", portProfile.TxPause,
		)
		out, err := NicctlExecute(args...)
		if err != nil {
			log_e.Errorf("Failed to update pause settings for port %s: %v", portIdentifier, err)
			updateAINICStatus("Failed", fmt.Sprintf("Failed to update pause settings for port %s: %v", portIdentifier, err))
			generateAINICK8sEvent(err, globals.K8EventAINICPortProfileApplyFailed)
			return false
		} else {
			log.Printf("%v", out)
		}
	}

	// Classification Type (global setting - no port specification needed)
	if portProfile.ClassificationType.Enum().String() != "CLASSIFICATION_TYPE_UNSPECIFIED" {
		out, err := NicctlExecute("update", "qos", "--classification-type", portProfile.ClassificationType.Enum().String())
		if err != nil {
			log_e.Errorf("Failed to update classification type: %v", err)
			updateAINICStatus("Failed", fmt.Sprintf("Failed to update classification type: %v", err))
			generateAINICK8sEvent(err, globals.K8EventAINICPortProfileApplyFailed)
			return false
		} else {
			log.Printf("%v", out)
		}
	}

	// DSCP to Priority (port-specific)
	for _, entry := range portProfile.DscpToPriority {
		dscp := strings.Join(entry.Dscp, ",")
		priority := fmt.Sprintf("%d", entry.Priority)

		var args []string
		if portIdentifier == "all" {
			args = []string{"update", "qos", "dscp-to-priority", "--dscp", dscp, "--priority", priority}
		} else {
			args = []string{"update", "qos", "-p", portIdentifier, "dscp-to-priority", "--dscp", dscp, "--priority", priority}
		}

		out, err := NicctlExecute(args...)
		if err != nil {
			log_e.Errorf("Failed to update DSCP to priority for port %s: %v", portIdentifier, err)
			updateAINICStatus("Failed", fmt.Sprintf("Failed to update DSCP to priority for port %s: %v", portIdentifier, err))
			generateAINICK8sEvent(err, globals.K8EventAINICPortProfileApplyFailed)
			return false
		} else {
			log.Printf("%v", out)
		}
	}

	// PFC (port-specific)
	if portProfile.Pfc != nil {
		var args []string
		if portIdentifier == "all" {
			args = []string{"update", "qos", "pfc", "--priority", fmt.Sprintf("%d", portProfile.Pfc.Priority), "--no-drop", portProfile.Pfc.NoDrop}
		} else {
			args = []string{"update", "qos", "-p", portIdentifier, "pfc", "--priority", fmt.Sprintf("%d", portProfile.Pfc.Priority), "--no-drop", portProfile.Pfc.NoDrop}
		}

		out, err := NicctlExecute(args...)
		if err != nil {
			log_e.Errorf("Failed to update PFC for port %s: %v", portIdentifier, err)
			updateAINICStatus("Failed", fmt.Sprintf("Failed to update PFC for port %s: %v", portIdentifier, err))
			generateAINICK8sEvent(err, globals.K8EventAINICPortProfileApplyFailed)
			return false
		} else {
			log.Printf("%v", out)
		}
	}

	// Scheduling (port-specific)
	if portProfile.Scheduling != nil {
		priorities := utils.IntsToCSV(portProfile.Scheduling.Priority)
		rates := utils.IntsToCSV(portProfile.Scheduling.RateLimit)
		dwrr := utils.IntsToCSV(portProfile.Scheduling.Dwrr)

		var args []string
		if portIdentifier == "all" {
			args = []string{"update", "qos", "scheduling",
				"--priority", priorities,
				"--rate-limit", rates,
				"--dwrr", dwrr}
		} else {
			args = []string{"update", "qos", "-p", portIdentifier, "scheduling",
				"--priority", priorities,
				"--rate-limit", rates,
				"--dwrr", dwrr}
		}

		out, err := NicctlExecute(args...)
		if err != nil {
			log_e.Errorf("Failed to update scheduling for port %s: %v", portIdentifier, err)
			updateAINICStatus("Failed", fmt.Sprintf("Failed to update scheduling for port %s: %v", portIdentifier, err))
			generateAINICK8sEvent(err, globals.K8EventAINICPortProfileApplyFailed)
			return false // Return early on scheduling failure
		} else {
			log.Printf("%v", out)
		}
	}

	// Port profile applied successfully - specific success events will be generated per NIC profile
	return true
}

func ApplyDcqcnProfile(profileName string, device string, dcqcnProfile *ainic_pb.DCQCNProfile, index int) bool {

	args := []string{
		"update", "dcqcn",
		"--roce-device", device,
		"--profile-id", fmt.Sprintf("%d", index),
	}

	// Add optional fields if present
	if dcqcnProfile.Disable {
		args = append(args, "--disable")
	}
	if dcqcnProfile.TokenBucketSize > 0 {
		args = append(args, "--token-bucket-size", fmt.Sprintf("%d", dcqcnProfile.TokenBucketSize))
	}
	if dcqcnProfile.RateIncreaseByteCount > 0 {
		args = append(args, "--rate-increase-byte-count", fmt.Sprintf("%d", dcqcnProfile.RateIncreaseByteCount))
	}
	if dcqcnProfile.ClampTargetRateEn {
		args = append(args, "--clamp-target-rate", "enable")
	}
	if dcqcnProfile.RateReduceMonitorPeriod > 0 {
		args = append(args, "--rate-reduce-monitor-period", fmt.Sprintf("%d", dcqcnProfile.RateReduceMonitorPeriod))
	}
	if dcqcnProfile.InitialAlphaValue > 0 {
		args = append(args, "--initial-alpha-value", fmt.Sprintf("%d", dcqcnProfile.InitialAlphaValue))
	}
	if dcqcnProfile.AlphaUpdateG > 0 {
		args = append(args, "--alpha-update-g", fmt.Sprintf("%d", dcqcnProfile.AlphaUpdateG))
	}
	if dcqcnProfile.AiRate > 0 {
		args = append(args, "--ai-rate", fmt.Sprintf("%d", dcqcnProfile.AiRate))
	}
	if dcqcnProfile.HaiRate > 0 {
		args = append(args, "--hai-rate", fmt.Sprintf("%d", dcqcnProfile.HaiRate))
	}
	if dcqcnProfile.AlphaUpdateInterval > 0 {
		args = append(args, "--alpha-update-interval", fmt.Sprintf("%d", dcqcnProfile.AlphaUpdateInterval))
	}
	if dcqcnProfile.MinRate > 0 {
		args = append(args, "--min-rate", fmt.Sprintf("%d", dcqcnProfile.MinRate))
	}
	if dcqcnProfile.RateIncreaseThreshold > 0 {
		args = append(args, "--rate-increase-threshold", fmt.Sprintf("%d", dcqcnProfile.RateIncreaseThreshold))
	}
	if dcqcnProfile.RateIncreaseInterval > 0 {
		args = append(args, "--rate-increase-interval", fmt.Sprintf("%d", dcqcnProfile.RateIncreaseInterval))
	}
	if dcqcnProfile.CnpDscp > 0 {
		args = append(args, "--cnp-dscp", fmt.Sprintf("%d", dcqcnProfile.CnpDscp))
	}

	// log.Printf("Executing: nicctl %s", strings.Join(args, " "))
	output, err := NicctlExecute(args...)
	if err != nil {
		log_e.Errorf("Command failed: %v", err)
		updateAINICStatus("Failed", fmt.Sprintf("Failed to apply DCQCN profile %s to device %s: %v", profileName, device, err))
		generateAINICK8sEventWithContext(err, globals.K8EventAINICDCQCNProfileApplyFailed, fmt.Sprintf("DCQCN Profile: %s, Device: %s", profileName, device))
		return false
	} else {
		log.Printf("%v", output)
		// Don't generate individual success events for each DCQCN profile to avoid spam
		return true
	}

}

func ApplyDCQCNConfigs(nicProfile *ainic_pb.NicProfile, nicToLifs map[string][]string, name string, dcqcnProfiles map[string]*ainic_pb.DCQCNProfile) bool {
	for i, dcqcn := range nicProfile.GetDcqcn() {
		log.Printf("DCQCN Entry #%d", i)
		log.Printf("  Match Filters DCQCN: %v", dcqcn.GetMatchFiltersDcqcn())
		log.Printf("  Profiles: %v", dcqcn.GetProfiles())

		var roceDeviceList []string
		// Check if MatchFiltersDcqcn has any entries before accessing
		if len(dcqcn.MatchFiltersDcqcn) == 0 {
			log_e.Errorf("DCQCN entry %d has no MatchFiltersDcqcn entries", i)
			updateAINICStatusForCard("unknown", name, "Failed", "No MatchFiltersDcqcn entries found", "ApplyDCQCNConfigs")
			generateAINICK8sEventWithContext(errors.New("no MatchFiltersDcqcn entries found"), globals.K8EventAINICNicctlCommandFailed, fmt.Sprintf("NIC Profile: %s, DCQCN Entry: %d", name, i))
			return false
		}

		device := dcqcn.MatchFiltersDcqcn[0].DevId
		if device == "all" {
			roceDeviceList = nicToLifs[name]
		} else {
			roceDeviceList = []string{device}
		}

		for _, device := range roceDeviceList {
			if device == "" {
				continue
			}
			log.Printf("========================= %s ===================", device)
			// Apply all DCQCN profiles to a ROCE device, 1 or 8 depending on workload or provider mode
			for index, profName := range dcqcn.GetProfiles() {
				if dcqcnProf, ok := dcqcnProfiles[profName]; ok {
					success := ApplyDcqcnProfile(profName, device, dcqcnProf, index+1)
					if !success {
						return false
					}
				}
			}
			log.Printf("========================= End of %s ===================", device)
		}
	}
	return true
}

func ParseAndApplyProfile(selectedProfile string, config *ainic_pb.Config, nicToCards map[string][]string, nicToLifs map[string][]string) bool {

	nodeProfile := config.NodeProfiles[selectedProfile]
	nicProfiles := nodeProfile.GetNicprofiles()

	log.Println("NIC Profiles:", nicProfiles)

	for _, name := range nicProfiles {
		nicProfile := config.NicProfiles[name]

		log.Printf("================ %s ================", name)
		log.Printf("Match Filters: %v", nicProfile.GetMatchFilters())
		log.Printf("Card Profile: %s", nicProfile.GetCardProfile())
		log.Printf("Config Profile: %s", nicProfile.GetConfigPreference())
		log.Printf("Port Profile: %v", nicProfile.GetPortProfile())
		log.Printf("VF Count: %v", nicProfile.GetVfCount())
		log.Printf("================ %s ================", name)

		portProfileMap := nicProfile.GetPortProfile()
		dcqcnProfiles := config.DcqcnProfiles

		// Apply port profiles to each port
		portSuccess := ApplyPortProfiles(portProfileMap, nicToCards, name, config.PortProfiles)
		if portSuccess {
			// Update status first
			updateAINICStatus("Success", fmt.Sprintf("Port profile applied successfully for NIC profile %s", name))
			// Update per-card status for port profiles
			for _, cardID := range nicToCards[name] {
				updateAINICStatusForCard(cardID, name, "Success", "Port profile applied successfully", "ApplyPortProfiles")
			}
			// Generate success event for port profile application
			generateAINICK8sEventWithContext(nil, globals.K8EventAINICPortProfileApplySuccess, fmt.Sprintf("NIC Profile: %s", name))
		} else {
			// Update status first
			updateAINICStatus("Failed", fmt.Sprintf("Port profile application failed for NIC profile %s", name))
			// Update per-card status for port profiles
			for _, cardID := range nicToCards[name] {
				updateAINICStatusForCard(cardID, name, "Failed", "Port profile application failed", "ApplyPortProfiles")
			}
			// Generate failure event for port profile application
			generateAINICK8sEventWithContext(errors.New("port profile application failed"), globals.K8EventAINICPortProfileApplyFailed, fmt.Sprintf("NIC Profile: %s", name))
		}

		// Apply DCQCN regardless of port profile application result
		dcqcnSuccess := ApplyDCQCNConfigs(nicProfile, nicToLifs, name, dcqcnProfiles)
		if dcqcnSuccess {
			// Update status first
			updateAINICStatus("Success", fmt.Sprintf("DCQCN profiles applied successfully for NIC profile %s", name))
			// Update per-card status for DCQCN profiles
			for _, cardID := range nicToCards[name] {
				updateAINICStatusForCard(cardID, name, "Success", "DCQCN profiles applied successfully", "ApplyDCQCNConfigs")
			}
			// Generate success event for DCQCN profile application
			generateAINICK8sEventWithContext(nil, globals.K8EventAINICDCQCNProfileApplySuccess, fmt.Sprintf("NIC Profile: %s", name))
		} else {
			// Update status first
			updateAINICStatus("Failed", fmt.Sprintf("DCQCN profiles application failed for NIC profile %s", name))
			// Update per-card status for DCQCN profiles
			for _, cardID := range nicToCards[name] {
				updateAINICStatusForCard(cardID, name, "Failed", "DCQCN profiles application failed", "ApplyDCQCNConfigs")
			}
			// Generate failure event for DCQCN profile application
			generateAINICK8sEventWithContext(errors.New("DCQCN profile application failed"), globals.K8EventAINICDCQCNProfileApplyFailed, fmt.Sprintf("NIC Profile: %s", name))
		}

		// Return false only if both port AND dcqcn fail
		if !portSuccess && !dcqcnSuccess {
			return false
		}
	}
	return true
}

package utils

import (
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/godbus/dbus/v5"
	log_e "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

const serviceDivider = "^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^"

type ServicePreState struct {
	Name      string // e.g. "amd-metrics-exporter.service"
	State     string // e.g. "active", "inactive", "failed", "not-loaded"
	Timestamp time.Time
	Comment   string
}

var PreStateDB = make(map[string]ServicePreState)

// connect to the system D-Bus
func getSystemdConn() (*dbus.Conn, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system bus: %v", err)
	}
	return conn, nil
}

// service control based on action (StartUnit or StopUnit)
func controlService(action, serviceName string) error {
	conn, err := getSystemdConn()
	if err != nil {
		return err
	}
	obj := conn.Object("org.freedesktop.systemd1", dbus.ObjectPath("/org/freedesktop/systemd1"))
	call := obj.Call("org.freedesktop.systemd1.Manager."+action+"Unit", 0, serviceName, "replace")

	if call.Err != nil {
		return fmt.Errorf("D-Bus call failed: %v", call.Err)
	}
	log.Printf("Service '%s' %s triggered.\n", serviceName, strings.ToLower(action))
	return nil
}

func StartService(name string) error {
	if UnitExists(name) && CheckUnitStatusHandler(name, "active") {
		log.Printf("Service %v already exists and in active state. Skipping restart!!", name)
		err := errors.New("service already exists and in active state")
		return err
	}
	return controlService("Start", name)
}

func StopService(name string) error {
	if !UnitExists(name) {
		log.Printf("Service %v does not exist. Skipping!!", name)
		err := errors.New("service does not exist")
		return err
	}
	return controlService("Stop", name)
}

func CleanupPreState() {
	log.Println("Cleaning up PreStateDB...")
	PreStateDB = make(map[string]ServicePreState)
	if len(PreStateDB) == 0 {
		log.Println("PreStateDB has been successfully emptied.")
	} else {
		log.Printf("Warning: PreStateDB still has %d entries.", len(PreStateDB))
	}
}

func StartServiceHandler(services []string) {
	log.Println(serviceDivider)
	log.Printf("ServicesList %v", services)
	for _, svc := range services {
		if !strings.HasSuffix(svc, ".service") {
			svc += ".service"
		}
		preState := PreStateDB[svc]
		if preState.State == "active" {
			log.Printf("Service %s prestate is active (status: %s), attempting restart", svc, preState.State)
		} else {
			log.Printf("Restarting service skipped for: %s (was %s at %s)\n", svc, preState.State, preState.Timestamp)
			continue
		}
		log.Printf("Restarting service: %s", svc)
		if err := StartService(svc); err != nil {
			log.Printf("Warning: Failed to start service %s: %v\n", svc, err)
		} else {
			log.Printf("Validating %v service status", svc)
			if CheckUnitStatusHandler(svc, "active") {
				log.Printf("Service %s (status: %s), successfully restarted", svc, "active")
			}
		}
	}
	CleanupPreState()
	log.Println(serviceDivider)
}

func StopServiceHandler(services []string) {
	log.Println(serviceDivider)
	log.Printf("ServicesList %v", services)
	for _, svc := range services {
		if !strings.HasSuffix(svc, ".service") {
			svc += ".service"
		}
		currStatus := CheckUnitStatus(svc)

		// write the previous service state only if it is not already recorded
		// this is to avoid overwriting the state during the partition retry
		if _, ok := PreStateDB[svc]; !ok {
			PreStateDB[svc] = ServicePreState{
				Name:      svc,
				State:     currStatus,
				Timestamp: time.Now(),
				Comment:   fmt.Sprintf("Service was %s before StopService", currStatus),
			}
		}

		// when determining whether to stop the service
		// we only want to look at the current state, not the pre-state
		if currStatus != "active" {
			log.Printf("Service %s is not active (status: %s), skipping stop", svc, currStatus)
			continue
		} else {
			log.Printf("Service %s current state is active (status: %s), attempting stop", svc, currStatus)
		}

		log.Printf("Stopping service: %s", svc)
		if err := StopService(svc); err != nil {
			log.Printf("Warning: Failed to stop service %s: %v\n", svc, err)
		} else {
			log.Printf("Validating %v service status", svc)
			if CheckUnitStatusHandler(svc, "not-loaded") {
				log.Printf("Service %s (status: %s), successfully stopped", svc, "not-loaded")
			}
		}
	}
	log.Println(serviceDivider)
}

// checking if a systemd unit exists
func UnitExists(unitName string) bool {
	// sleep for 2 seconds to determine the status
	time.Sleep(2 * time.Second)
	log.Printf("Checking if %v exists", unitName)
	conn, err := dbus.SystemBus()
	if err != nil {
		return false
	}

	systemd := conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1")
	var unitPath dbus.ObjectPath

	err = systemd.Call("org.freedesktop.systemd1.Manager.GetUnit", 0, unitName).Store(&unitPath)
	return err == nil
}

// check service status
func CheckUnitStatus(name string) string {
	// sleep for 10 seconds to determine the status
	time.Sleep(10 * time.Second)
	conn, err := getSystemdConn()
	if err != nil {
		log_e.Errorf("err: %+v", err)
		return ""
	}
	manager := conn.Object("org.freedesktop.systemd1", dbus.ObjectPath("/org/freedesktop/systemd1"))
	var unitPath dbus.ObjectPath

	err = manager.Call("org.freedesktop.systemd1.Manager.GetUnit", 0, name).Store(&unitPath)
	if err != nil {
		if dbusErr, ok := err.(dbus.Error); ok {
			if dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit" {
				return "not-loaded"
			}
		}
		log_e.Errorf("failed to get unit: %v", err)
		return ""
	}

	unit := conn.Object("org.freedesktop.systemd1", unitPath)
	variant, err := unit.GetProperty("org.freedesktop.systemd1.Unit.ActiveState")
	if err != nil {
		log_e.Errorf("failed to get ActiveState: %v", err)
		return ""
	}

	activeState, ok := variant.Value().(string)
	if !ok {
		log_e.Errorf("unexpected type for ActiveState")
		return ""
	}

	return activeState
}

func CheckUnitStatusHandler(svc string, exp_status string) bool {
	status := CheckUnitStatus(svc)
	if status != exp_status {
		log.Printf("Service %s (status: %s), (expected status: %v)", svc, status, exp_status)
		return false
	}
	return true
}

func IsKubernetes() bool {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		log.Println("Running inside a Kubernetes pod")
		return true
	} else {
		log.Println("Running as a service (debian)")
	}
	return false
}

func IntsToCSV(values []uint32) string {
	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(strs, ",")
}

// FileChangeCallback defines the function signature for file change callbacks
type FileChangeCallback func()

func StartFileWatcher(filePath string, callback FileChangeCallback) {
	log.Printf("Adding file watcher for %v", filePath)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Failed to create file watcher: %v", err)
		return
	}
	defer watcher.Close()

	// Check if file exists, retry for 30 minutes with 60s intervals
	retryInterval := 30 * time.Second
	timeout := 30 * time.Minute
	deadline := time.Now().Add(timeout)

	for {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if time.Now().After(deadline) {
				log.Printf("File %s does not exist after 30 minutes of polling, giving up on file watcher setup", filePath)
				return
			}
			log.Printf("File %s does not exist, will retry in 30 seconds (timeout in %.0f minutes)",
				filePath, time.Until(deadline).Minutes())
			time.Sleep(retryInterval)
			continue
		}
		// File exists, proceed with setup
		log.Printf("File %s found, proceeding with file watcher setup", filePath)
		break
	}

	// Add the JSON file to the watcher
	err = watcher.Add(filePath)
	if err != nil {
		log.Printf("Failed to add file to watcher: %v", err)
		return
	}

	log.Printf("Starting file watcher for %v", filePath)

	// Watch for changes
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					log.Print("Event channel closed")
					return
				}
				if event.Has(fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename) {
					log.Print("Detected changes in config file, triggering callback...")
					callback()
				} else {
					log.Printf("Unhandled event: %v", event)
				}
				// Re-add the file to the watcher after changes
				watcher.Remove(filePath)
				watcher.Add(filePath)
			case err, ok := <-watcher.Errors:
				if !ok {
					log.Print("Error channel closed")
					return
				}
				log.Printf("File watcher error: %v", err)
			}
		}
	}()

	// Keep the program running
	<-make(chan struct{})
}

// NodeLabelChangeCallback defines the function signature for node label change callbacks
type NodeLabelChangeCallback func()

func PrintAndApplyLabelChanges(oldLabels, newLabels map[string]string, labelKey string, onLabelChange NodeLabelChangeCallback) {
	// Check for added or updated labels
	for key, newVal := range newLabels {
		if key == labelKey && newVal != "" {
			if oldVal, exists := oldLabels[key]; !exists || oldVal != newVal {
				log.Printf("\nNEW TRIGGER ALERT FROM NODE LABELS\n")
				log.Printf("Label changed: %s\nOld value: %s\nNew value: %s\n", key, oldVal, newVal)
				onLabelChange()
			}
		}
	}

	// Check for removed labels
	for key, oldVal := range oldLabels {
		if _, exists := newLabels[key]; !exists {
			if key == labelKey {
				log.Printf("Label removed: %s\nOld value: %s\n", key, oldVal)
			}
		}
	}
}

func NodeLabelWatcher(kc interface{}, nodeName string, labelKey string, onLabelChange NodeLabelChangeCallback) {
	// Type assertion to get the GetNodeInformer method
	type K8sClient interface {
		GetNodeInformer(string) cache.SharedIndexInformer
	}

	k8sClient := kc.(K8sClient)
	nodeInformer := k8sClient.GetNodeInformer(nodeName)

	// Set up event handlers for the node informer
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNode := oldObj.(*v1.Node)
			newNode := newObj.(*v1.Node)
			if !reflect.DeepEqual(oldNode.Labels, newNode.Labels) {
				PrintAndApplyLabelChanges(oldNode.Labels, newNode.Labels, labelKey, onLabelChange)
			}
		},
	})

	// Start the informer
	stopCh := make(chan struct{})
	defer close(stopCh)

	go func() {
		// Creating a timer to prevent blockage of code execution
		timer := time.NewTimer(100 * time.Second)
		<-timer.C
		// Stop the Node Informer after the timer expires
	}()

	go nodeInformer.Run(stopCh)

	// Wait for the informer to sync
	if !cache.WaitForCacheSync(stopCh, nodeInformer.HasSynced) {
		log_e.Errorf("Failed to sync informers")
	}

	log.Print("Node Informer started and will run for 100 seconds.")
	// Keep the function running
	<-make(chan struct{})
}

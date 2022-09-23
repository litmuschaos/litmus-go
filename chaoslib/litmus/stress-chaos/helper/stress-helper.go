package helper

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/cgroups"
	cgroupsv2 "github.com/containerd/cgroups/v2"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/stress-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//list of cgroups in a container
var (
	cgroupSubsystemList = []string{"cpu", "memory", "systemd", "net_cls",
		"net_prio", "freezer", "blkio", "perf_event", "devices", "cpuset",
		"cpuacct", "pids", "hugetlb",
	}
)

var (
	err           error
	inject, abort chan os.Signal
)

const (
	// ProcessAlreadyFinished contains error code when process is finished
	ProcessAlreadyFinished = "os: process already finished"
	// ProcessAlreadyKilled contains error code when process is already killed
	ProcessAlreadyKilled = "no such process"
)

// Helper injects the stress chaos
func Helper(clients clients.ClientSets) {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}
	resultDetails := types.ResultDetails{}

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	//Fetching all the ENV passed for the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	getENV(&experimentsDetails)

	// Intialise the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	if err := prepareStressChaos(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails); err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

//prepareStressChaos contains the chaos preparation and injection steps
func prepareStressChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(1)
	default:

		containerID, err := common.GetContainerID(experimentsDetails.AppNS, experimentsDetails.TargetPods, experimentsDetails.TargetContainer, clients)
		if err != nil {
			return err
		}
		// extract out the pid of the target container
		targetPID, err := getPID(experimentsDetails, containerID)
		if err != nil {
			return err
		}

		// record the event inside chaosengine
		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		cgroupManager, err := getCGroupManager(int(targetPID), containerID)
		if err != nil {
			return errors.Errorf("fail to get the cgroup manager, err: %v", err)
		}

		// get stressors in list format
		stressorList := prepareStressor(experimentsDetails)
		if len(stressorList) == 0 {
			return errors.Errorf("fail to prepare stressor for %v experiment", experimentsDetails.ExperimentName)
		}
		stressors := strings.Join(stressorList, " ")
		stressCommand := "pause nsutil -t " + strconv.Itoa(targetPID) + " -p -- " + stressors
		log.Infof("[Info]: starting process: %v", stressCommand)

		// launch the stress-ng process on the target container in paused mode
		cmd := exec.Command("/bin/bash", "-c", stressCommand)
		// enables the process group id
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var buf bytes.Buffer
		cmd.Stdout = &buf
		err = cmd.Start()
		if err != nil {
			return errors.Errorf("fail to start the stress process %v, err: %v", stressCommand, err)
		}

		// watching for the abort signal and revert the chaos if an abort signal is received
		go abortWatcher(cmd.Process.Pid, resultDetails.Name, chaosDetails.ChaosNamespace, experimentsDetails.TargetPods)

		// add the stress process to the cgroup of target container
		if err = addProcessToCgroup(cmd.Process.Pid, cgroupManager); err != nil {
			if killErr := cmd.Process.Kill(); killErr != nil {
				return errors.Errorf("stressors failed killing %v process, err: %v", cmd.Process.Pid, killErr)
			}
			return errors.Errorf("fail to add the stress process into target container cgroup, err: %v", err)
		}

		log.Info("[Info]: Sending signal to resume the stress process")
		// wait for the process to start before sending the resume signal
		// TODO: need a dynamic way to check the start of the process
		time.Sleep(700 * time.Millisecond)

		// remove pause and resume or start the stress process
		if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
			return errors.Errorf("fail to remove pause and start the stress process: %v", err)
		}

		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", experimentsDetails.TargetPods); err != nil {
			return err
		}

		log.Info("[Wait]: Waiting for chaos completion")
		// channel to check the completion of the stress process
		done := make(chan error)
		go func() { done <- cmd.Wait() }()

		// check the timeout for the command
		// Note: timeout will occur when process didn't complete even after 10s of chaos duration
		timeout := time.After((time.Duration(experimentsDetails.ChaosDuration) + 30) * time.Second)

		select {
		case <-timeout:
			// the stress process gets timeout before completion
			log.Infof("[Chaos] The stress process is not yet completed after the chaos duration of %vs", experimentsDetails.ChaosDuration+30)
			log.Info("[Timeout]: Killing the stress process")
			if err = terminateProcess(cmd.Process.Pid); err != nil {
				return err
			}
			if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", experimentsDetails.TargetPods); err != nil {
				return err
			}
			return nil
		case err := <-done:
			if err != nil {
				err, ok := err.(*exec.ExitError)
				if ok {
					status := err.Sys().(syscall.WaitStatus)
					if status.Signaled() && status.Signal() == syscall.SIGKILL {
						// wait for the completion of abort handler
						time.Sleep(10 * time.Second)
						return errors.Errorf("process stopped with SIGKILL signal")
					}
				}
				return errors.Errorf("process exited before the actual cleanup, err: %v", err)
			}
			log.Info("[Info]: Chaos injection completed")
			if err := terminateProcess(cmd.Process.Pid); err != nil {
				return err
			}
			if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", experimentsDetails.TargetPods); err != nil {
				return err
			}
		}
	}
	return nil
}

//terminateProcess will remove the stress process from the target container after chaos completion
func terminateProcess(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		if strings.Contains(err.Error(), ProcessAlreadyKilled) || strings.Contains(err.Error(), ProcessAlreadyFinished) {
			return nil
		}
		return err
	}
	log.Info("[Info]: Stress process removed successfully")
	return nil
}

//prepareStressor will set the required stressors for the given experiment
func prepareStressor(experimentDetails *experimentTypes.ExperimentDetails) []string {

	stressArgs := []string{
		"stress-ng",
		"--timeout",
		strconv.Itoa(experimentDetails.ChaosDuration) + "s",
	}

	switch experimentDetails.StressType {
	case "pod-cpu-stress":

		log.InfoWithValues("[Info]: Details of Stressor:", logrus.Fields{
			"CPU Core": experimentDetails.CPUcores,
			"CPU Load": experimentDetails.CPULoad,
			"Timeout":  experimentDetails.ChaosDuration,
		})
		stressArgs = append(stressArgs, "--cpu "+experimentDetails.CPUcores)
		stressArgs = append(stressArgs, " --cpu-load "+experimentDetails.CPULoad)

	case "pod-memory-stress":

		log.InfoWithValues("[Info]: Details of Stressor:", logrus.Fields{
			"Number of Workers":  experimentDetails.NumberOfWorkers,
			"Memory Consumption": experimentDetails.MemoryConsumption,
			"Timeout":            experimentDetails.ChaosDuration,
		})
		stressArgs = append(stressArgs, "--vm "+experimentDetails.NumberOfWorkers+" --vm-bytes "+experimentDetails.MemoryConsumption+"M")

	case "pod-io-stress":
		var hddbytes string
		if experimentDetails.FilesystemUtilizationBytes == "0" {
			if experimentDetails.FilesystemUtilizationPercentage == "0" {
				hddbytes = "10%"
				log.Info("Neither of FilesystemUtilizationPercentage or FilesystemUtilizationBytes provided, proceeding with a default FilesystemUtilizationPercentage value of 10%")
			} else {
				hddbytes = experimentDetails.FilesystemUtilizationPercentage + "%"
			}
		} else {
			if experimentDetails.FilesystemUtilizationPercentage == "0" {
				hddbytes = experimentDetails.FilesystemUtilizationBytes + "G"
			} else {
				hddbytes = experimentDetails.FilesystemUtilizationPercentage + "%"
				log.Warn("Both FsUtilPercentage & FsUtilBytes provided as inputs, using the FsUtilPercentage value to proceed with stress exp")
			}
		}
		log.InfoWithValues("[Info]: Details of Stressor:", logrus.Fields{
			"io":                experimentDetails.NumberOfWorkers,
			"hdd":               experimentDetails.NumberOfWorkers,
			"hdd-bytes":         hddbytes,
			"Timeout":           experimentDetails.ChaosDuration,
			"Volume Mount Path": experimentDetails.VolumeMountPath,
		})
		if experimentDetails.VolumeMountPath == "" {
			stressArgs = append(stressArgs, "--io "+experimentDetails.NumberOfWorkers+" --hdd "+experimentDetails.NumberOfWorkers+" --hdd-bytes "+hddbytes)
		} else {
			stressArgs = append(stressArgs, "--io "+experimentDetails.NumberOfWorkers+" --hdd "+experimentDetails.NumberOfWorkers+" --hdd-bytes "+hddbytes+" --temp-path "+experimentDetails.VolumeMountPath)
		}
		if experimentDetails.CPUcores != "0" {
			stressArgs = append(stressArgs, "--cpu %v", experimentDetails.CPUcores)
		}

	default:
		log.Fatalf("stressor for %v experiment is not suported", experimentDetails.ExperimentName)
	}
	return stressArgs
}

//getPID extract out the PID of the target container
func getPID(experimentDetails *experimentTypes.ExperimentDetails, containerID string) (int, error) {
	var PID int

	switch experimentDetails.ContainerRuntime {
	case "docker":
		host := "unix://" + experimentDetails.SocketPath
		// deriving pid from the inspect out of target container
		out, err := exec.Command("sudo", "docker", "--host", host, "inspect", containerID).CombinedOutput()
		if err != nil {
			log.Error(fmt.Sprintf("[docker]: Failed to run docker inspect: %s", string(out)))
			return 0, err
		}

		// parsing data from the json output of inspect command
		PID, err = parsePIDFromJSON(out, experimentDetails.ContainerRuntime)
		if err != nil {
			log.Error(fmt.Sprintf("[docker]: Failed to parse json from docker inspect output: %s", string(out)))
			return 0, err
		}

	case "containerd", "crio":
		// deriving pid from the inspect out of target container
		endpoint := "unix://" + experimentDetails.SocketPath
		out, err := exec.Command("sudo", "crictl", "-i", endpoint, "-r", endpoint, "inspect", containerID).CombinedOutput()
		if err != nil {
			log.Error(fmt.Sprintf("[cri]: Failed to run crictl: %s", string(out)))
			return 0, err
		}

		// parsing data from the json output of inspect command
		PID, err = parsePIDFromJSON(out, experimentDetails.ContainerRuntime)
		if err != nil {
			log.Errorf(fmt.Sprintf("[cri]: Failed to parse json from crictl output: %s", string(out)))
			return 0, err
		}
	default:
		return 0, errors.Errorf("%v container runtime not suported", experimentDetails.ContainerRuntime)
	}

	log.Info(fmt.Sprintf("[Info]: Container ID=%s has process PID=%d", containerID, PID))

	return PID, nil
}

//pidPath will get the pid path of the container
func pidPath(pid int) cgroups.Path {
	processPath := "/proc/" + strconv.Itoa(pid) + "/cgroup"
	paths, err := parseCgroupFile(processPath)
	if err != nil {
		return getErrorPath(errors.Wrapf(err, "parse cgroup file %s", processPath))
	}
	return getExistingPath(paths, pid, "")
}

//parseCgroupFile will read and verify the cgroup file entry of a container
func parseCgroupFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Errorf("unable to parse cgroup file: %v", err)
	}
	defer file.Close()
	return parseCgroupFromReader(file)
}

//parseCgroupFromReader will parse the cgroup file from the reader
func parseCgroupFromReader(r io.Reader) (map[string]string, error) {
	var (
		cgroups = make(map[string]string)
		s       = bufio.NewScanner(r)
	)
	for s.Scan() {
		var (
			text  = s.Text()
			parts = strings.SplitN(text, ":", 3)
		)
		if len(parts) < 3 {
			return nil, errors.Errorf("invalid cgroup entry: %q", text)
		}
		for _, subs := range strings.Split(parts[1], ",") {
			if subs != "" {
				cgroups[subs] = parts[2]
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, errors.Errorf("buffer scanner failed: %v", err)
	}

	return cgroups, nil
}

//getExistingPath will be used to get the existing valid cgroup path
func getExistingPath(paths map[string]string, pid int, suffix string) cgroups.Path {
	for n, p := range paths {
		dest, err := getCgroupDestination(pid, n)
		if err != nil {
			return getErrorPath(err)
		}
		rel, err := filepath.Rel(dest, p)
		if err != nil {
			return getErrorPath(err)
		}
		if rel == "." {
			rel = dest
		}
		paths[n] = filepath.Join("/", rel)
	}
	return func(name cgroups.Name) (string, error) {
		root, ok := paths[string(name)]
		if !ok {
			if root, ok = paths[fmt.Sprintf("name=%s", name)]; !ok {
				return "", cgroups.ErrControllerNotActive
			}
		}
		if suffix != "" {
			return filepath.Join(root, suffix), nil
		}
		return root, nil
	}
}

//getErrorPath will give the invalid cgroup path
func getErrorPath(err error) cgroups.Path {
	return func(_ cgroups.Name) (string, error) {
		return "", err
	}
}

//getCgroupDestination will validate the subsystem with the mountpath in container mountinfo file.
func getCgroupDestination(pid int, subsystem string) (string, error) {
	mountinfoPath := fmt.Sprintf("/proc/%d/mountinfo", pid)
	file, err := os.Open(mountinfoPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	s := bufio.NewScanner(file)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		for _, opt := range strings.Split(fields[len(fields)-1], ",") {
			if opt == subsystem {
				return fields[3], nil
			}
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	return "", errors.Errorf("no destination found for %v ", subsystem)
}

//findValidCgroup will be used to get a valid cgroup path
func findValidCgroup(path cgroups.Path, target string) (string, error) {
	for _, subsystem := range cgroupSubsystemList {
		path, err := path(cgroups.Name(subsystem))
		if err != nil {
			log.Errorf("fail to retrieve the cgroup path, subsystem: %v, target: %v, err: %v", subsystem, target, err)
			continue
		}
		if strings.Contains(path, target) {
			return path, nil
		}
	}
	return "", errors.Errorf("never found valid cgroup for %s", target)
}

//parsePIDFromJSON extract the pid from the json output
func parsePIDFromJSON(j []byte, runtime string) (int, error) {
	var pid int
	switch runtime {
	case "docker":
		// in docker, pid is present inside state.pid attribute of inspect output
		var resp []common.DockerInspectResponse
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, err
		}
		pid = resp[0].State.PID
	case "containerd":
		var resp common.CrictlInspectResponse
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, err
		}
		pid = resp.Info.PID

	case "crio":
		var info common.InfoDetails
		if err := json.Unmarshal(j, &info); err != nil {
			return 0, err
		}
		pid = info.PID
		if pid == 0 {
			var resp common.CrictlInspectResponse
			if err := json.Unmarshal(j, &resp); err != nil {
				return 0, err
			}
			pid = resp.Info.PID
		}
	default:
		return 0, errors.Errorf("[cri]: No supported container runtime, runtime: %v", runtime)
	}
	if pid == 0 {
		return 0, errors.Errorf("[cri]: No running target container found, pid: %d", pid)
	}

	return pid, nil
}

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.AppNS = types.Getenv("APP_NAMESPACE", "")
	experimentDetails.TargetContainer = types.Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = types.Getenv("APP_POD", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
	experimentDetails.CPUcores = types.Getenv("CPU_CORES", "")
	experimentDetails.CPULoad = types.Getenv("CPU_LOAD", "")
	experimentDetails.FilesystemUtilizationPercentage = types.Getenv("FILESYSTEM_UTILIZATION_PERCENTAGE", "")
	experimentDetails.FilesystemUtilizationBytes = types.Getenv("FILESYSTEM_UTILIZATION_BYTES", "")
	experimentDetails.NumberOfWorkers = types.Getenv("NUMBER_OF_WORKERS", "")
	experimentDetails.MemoryConsumption = types.Getenv("MEMORY_CONSUMPTION", "")
	experimentDetails.VolumeMountPath = types.Getenv("VOLUME_MOUNT_PATH", "")
	experimentDetails.StressType = types.Getenv("STRESS_TYPE", "")
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(targetPID int, resultName, chaosNS, targetPodName string) {

	<-abort

	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("[Abort]: Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err = terminateProcess(targetPID); err != nil {
			log.Errorf("unable to kill stress process, err :%v", err)
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	if err = result.AnnotateChaosResult(resultName, chaosNS, "reverted", "pod", targetPodName); err != nil {
		log.Errorf("unable to annotate the chaosresult, err :%v", err)
	}
	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}

// getCGroupManager will return the cgroup for the given pid of the process
func getCGroupManager(pid int, containerID string) (interface{}, error) {
	if cgroups.Mode() == cgroups.Unified {
		groupPath, err := cgroupsv2.PidGroupPath(pid)
		if err != nil {
			return nil, errors.Errorf("Error in getting groupPath, %v", err)
		}

		cgroup2, err := cgroupsv2.LoadManager("/sys/fs/cgroup", groupPath)
		if err != nil {
			return nil, errors.Errorf("Error loading cgroup v2 manager, %v", err)
		}
		return cgroup2, nil
	}
	path := pidPath(pid)
	cgroup, err := findValidCgroup(path, containerID)
	if err != nil {
		return nil, errors.Errorf("fail to get cgroup, err: %v", err)
	}
	cgroup1, err := cgroups.Load(cgroups.V1, cgroups.StaticPath(cgroup))
	if err != nil {
		return nil, errors.Errorf("fail to load the cgroup, err: %v", err)
	}

	return cgroup1, nil
}

// addProcessToCgroup will add the process to cgroup
// By default it will add to v1 cgroup
func addProcessToCgroup(pid int, control interface{}) error {
	if cgroups.Mode() == cgroups.Unified {
		var cgroup1 = control.(*cgroupsv2.Manager)
		return cgroup1.AddProc(uint64(pid))
	}
	var cgroup1 = control.(cgroups.Cgroup)
	return cgroup1.Add(cgroups.Process{Pid: pid})
}

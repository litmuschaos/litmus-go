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
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/stress-chaos/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/stress-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	pidTypes "github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//list of cgroups in a container
var (
	cgroupSubsystemList = []string{"cpu", "memory", "systemd", "net_cls",
		"net_prio", "freezer", "blkio", "perf_event", "devices",
		"cpuset", "cpuacct", "pids", "hugetlb"}
)

var (
	err           error
	inject, abort chan os.Signal
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

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	//Fetching all the ENV passed for the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	getENV(&experimentsDetails)

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

	containerID, err := getContainerID(experimentsDetails, clients)
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

	//get the pid path and check cgroup
	path := pidPath(int(targetPID))
	cgroup, err := findValidCgroup(path, containerID)
	if err != nil {
		return errors.Errorf("fail to get cgroup, err: %v", err)
	}

	// load the existing cgroup
	control, err := cgroups.Load(cgroups.V1, cgroups.StaticPath(cgroup))
	if err != nil {
		return errors.Errorf("fail to load the cgroup, err: %v", err)
	}

	// get stressors list format
	stressorList := prepareStressor(experimentsDetails)
	if len(stressorList) == 0 {
		return errors.Errorf("fail to prepare stressor for %v experiment", experimentsDetails.ExperimentName)
	}
	stressors := strings.Join(stressorList, " ")
	stressCommand := "pause nsutil -t " + strconv.Itoa(targetPID) + " -p -- " + stressors
	log.Infof("[Info]: starting process: %v", stressCommand)

	// launch the stress-ng process on the target container in paused mode
	cmd := exec.Command("/bin/bash", "-c", stressCommand)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	err = cmd.Start()
	if err != nil {
		return errors.Errorf("fail to start the stress process %v, err: %v", stressCommand, err)
	}

	// watching for the abort signal and revert the chaos if an abort signal is received
	go abortWatcher(cmd.Process.Pid)

	// add the stress process to the cgroup of target container
	if err = control.Add(cgroups.Process{Pid: cmd.Process.Pid}); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			return errors.Errorf("stressors failed killing %v process, err: %v", cmd.Process.Pid, killErr)
		}
		return errors.Errorf("fail to add the stress process into target container cgroup, err: %v", err)
	}

	log.Info("[Info]: Sending signal to resume the stress process")
	// wait for the process to start before sending the resume signal
	time.Sleep(700 * time.Millisecond)

	// remove pause and resume or start the stress process
	if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
		return errors.Errorf("fail to remove pause and start the stress process: %v", err)
	}

	log.Info("[Wait]: Waiting for chaos completion")
	// channel to check the completion of the stress process
	done := make(chan error)
	go func() { done <- cmd.Wait() }()

	// check the timeout for the command
	timeout := time.After((time.Duration(experimentsDetails.ChaosDuration) + 10) * time.Second)

	select {
	case <-timeout:
		// the stress process gets timeout before completion
		log.Infof("[Timeout] Stress output: %v", buf.String())
		log.Info("[Cleaup]: Kill the stress process")
		terminateProcess(cmd.Process.Pid)
		return errors.Errorf("the stress process is timeout after %vs", experimentsDetails.ChaosDuration+10)
	case err := <-done:
		if err != nil {
			err, ok := err.(*exec.ExitError)
			if ok {
				status := err.Sys().(syscall.WaitStatus)
				if status.Signaled() && status.Signal() == syscall.SIGTERM {
					return errors.Errorf("process stopped with SIGTERM signal")
				}
			}
			return errors.Errorf("error process exited accidentally", err)
		}
		log.Info("[Info]: Chaos injection completed")
		terminateProcess(cmd.Process.Pid)
	}
	return nil
}

//terminateProcess will remove the stress process from the target container after chaos completion
func terminateProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return errors.Errorf("unreachable path, err: %v", err)
	}
	if err = process.Signal(syscall.SIGTERM); err != nil && err.Error() != "os: process already finished" {
		return errors.Errorf("error while killing process", err)
	}
	log.Info("[Info]: Stress process removed sucessfully")
	return nil
}

//prepareStressor will set the required stressors for the given experiment
func prepareStressor(experimentDetails *experimentTypes.ExperimentDetails) []string {

	stressArgs := []string{
		"stress-ng",
		"--timeout",
		strconv.Itoa(experimentDetails.ChaosDuration) + "s",
	}

	switch experimentDetails.ExperimentName {
	case "pod-cpu-hog":

		log.InfoWithValues("[Info]: Details of Stressor:", logrus.Fields{
			"CPU Core": experimentDetails.CPUcores,
			"Timeout":  experimentDetails.ChaosDuration,
		})
		stressArgs = append(stressArgs, "--cpu "+strconv.Itoa(experimentDetails.CPUcores))
		return stressArgs

	case "pod-memory-hog":

		log.InfoWithValues("[Info]: Details of Stressor:", logrus.Fields{
			"Number of Workers":  experimentDetails.NumberOfWorkers,
			"Memory Consumption": experimentDetails.MemoryConsumption,
			"Timeout":            experimentDetails.ChaosDuration,
		})
		stressArgs = append(stressArgs, "--vm "+strconv.Itoa(experimentDetails.NumberOfWorkers)+" --vm-bytes "+strconv.Itoa(experimentDetails.MemoryConsumption)+"M")
		return stressArgs

	case "pod-io-stress":
		var hddbytes string
		if experimentDetails.FilesystemUtilizationBytes == 0 {
			if experimentDetails.FilesystemUtilizationPercentage == 0 {
				hddbytes = "10%"
				log.Info("Neither of FilesystemUtilizationPercentage or FilesystemUtilizationBytes provided, proceeding with a default FilesystemUtilizationPercentage value of 10%")
			} else {
				hddbytes = strconv.Itoa(experimentDetails.FilesystemUtilizationPercentage) + "%"
			}
		} else {
			if experimentDetails.FilesystemUtilizationPercentage == 0 {
				hddbytes = strconv.Itoa(experimentDetails.FilesystemUtilizationBytes) + "G"
			} else {
				hddbytes = strconv.Itoa(experimentDetails.FilesystemUtilizationPercentage) + "%"
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
			stressArgs = append(stressArgs, "--cpu 1 --io "+strconv.Itoa(experimentDetails.NumberOfWorkers)+" --hdd "+strconv.Itoa(experimentDetails.NumberOfWorkers)+" --hdd-bytes "+hddbytes)
		} else {
			stressArgs = append(stressArgs, "--cpu 1 --io "+strconv.Itoa(experimentDetails.NumberOfWorkers)+" --hdd "+strconv.Itoa(experimentDetails.NumberOfWorkers)+" --hdd-bytes "+hddbytes+" --temp-path "+experimentDetails.VolumeMountPath)
		}
		return stressArgs

	default:
		log.Fatalf("stressor for %v experiment is not suported", experimentDetails.ExperimentName)
	}
	return nil
}

//getContainerID extract out the container id of the target container
func getContainerID(experimentDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {

	var containerID string
	switch strings.ToLower(experimentDetails.ContainerRuntime) {
	case "docker":
		host := "unix://" + experimentDetails.SocketPath
		// deriving the container id of the target container
		cmd := "sudo docker --host " + host + " ps | grep k8s_" + experimentDetails.TargetContainer + "_" + experimentDetails.TargetPods + "_" + experimentDetails.AppNS + " | awk '{print $1}'"
		out, err := exec.Command("/bin/sh", "-c", cmd).CombinedOutput()
		if err != nil {
			log.Error(fmt.Sprintf("[docker]: Failed to run docker ps command: %s", string(out)))
			return "", err
		}
		containerID = strings.TrimSpace(string(out))
	case "containerd", "crio":
		pod, err := clients.KubeClient.CoreV1().Pods(experimentDetails.AppNS).Get(experimentDetails.TargetPods, v1.GetOptions{})
		if err != nil {
			return "", err
		}
		// filtering out the container id from the details of containers inside containerStatuses of the given pod
		// container id is present in the form of <runtime>://<container-id>
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name == experimentDetails.TargetContainer {
				containerID = strings.Split(container.ContainerID, "//")[1]
				break
			}
		}
	default:
		return "", errors.Errorf("%v container runtime not suported", experimentDetails.ContainerRuntime)
	}
	log.Infof("containerid: %v", containerID)

	return containerID, nil
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
	return "", errors.Errorf("never found desct for %v ", subsystem)
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
	// namespaces are present inside `info.runtimeSpec.linux.namespaces` of inspect output
	// linux namespace of type network contains pid, in the form of `/proc/<pid>/ns/net`
	switch runtime {
	case "docker":
		// in docker, pid is present inside state.pid attribute of inspect output
		var resp []pidTypes.DockerInspectResponse
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, err
		}
		pid = resp[0].State.PID
	case "containerd":
		var resp pidTypes.CrictlInspectResponse
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, err
		}
		pid = resp.Info.PID

	case "crio":
		var info pidTypes.InfoDetails
		if err := json.Unmarshal(j, &info); err != nil {
			return 0, err
		}
		pid = info.PID
		if pid == 0 {
			var resp pidTypes.CrictlInspectResponse
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
	experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.AppNS = common.Getenv("APP_NS", "")
	experimentDetails.TargetContainer = common.Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = common.Getenv("APP_POD", "")
	experimentDetails.AppLabel = common.Getenv("APP_LABEL", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = common.Getenv("CHAOS_ENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = common.Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = common.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.SocketPath = common.Getenv("SOCKET_PATH", "")
	experimentDetails.CPUcores, _ = strconv.Atoi(common.Getenv("CPU_CORES", ""))
	experimentDetails.FilesystemUtilizationPercentage, _ = strconv.Atoi(common.Getenv("FILESYSTEM_UTILIZATION_PERCENTAGE", ""))
	experimentDetails.FilesystemUtilizationBytes, _ = strconv.Atoi(common.Getenv("FILESYSTEM_UTILIZATION_BYTES", ""))
	experimentDetails.NumberOfWorkers, _ = strconv.Atoi(common.Getenv("NUMBER_OF_WORKERS", ""))
	experimentDetails.MemoryConsumption, _ = strconv.Atoi(common.Getenv("MEMORY_CONSUMPTION", ""))
	experimentDetails.VolumeMountPath = common.Getenv("VOLUME_MOUNT_PATH", "")
}

// abortWatcher continuosly watch for the abort signals
func abortWatcher(targetPID int) {

	for {
		select {
		case <-abort:
			log.Info("[Abort]: Chaos Revert Started")
			log.Info("[Chaos]: Killing process started because of terminated signal received")
			// retry thrice for the chaos revert
			retry := 3
			for retry > 0 {
				if err = terminateProcess(targetPID); err != nil {
					log.Errorf("unable to kill stress process, err :%v", err)
				}
				retry--
				time.Sleep(1 * time.Second)
			}
			log.Info("[Abort]: Chaos Revert Completed")
			os.Exit(1)
		}
	}
}

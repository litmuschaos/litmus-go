package helper

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
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
	chaosDetails.Phase = types.ChaosInjectPhase

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	if err := prepareStressChaos(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails); err != nil {
		// update failstep inside chaosresult
		if resultErr := result.UpdateFailedStepFromHelper(&resultDetails, &chaosDetails, clients, err); resultErr != nil {
			log.Fatalf("helper pod failed, err: %v, resultErr: %v", err, resultErr)
		}
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

//prepareStressChaos contains the chaos preparation and injection steps
func prepareStressChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {
	// get stressors in list format
	stressorList := prepareStressor(experimentsDetails)
	if len(stressorList) == 0 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: chaosDetails.ChaosPodName, Reason: "fail to prepare stressors"}
	}
	stressors := strings.Join(stressorList, " ")

	targetList, err := common.ParseTargets(chaosDetails.ChaosPodName)
	if err != nil {
		return stacktrace.Propagate(err, "could not parse targets")
	}

	var targets []targetDetails

	for _, t := range targetList.Target {
		td := targetDetails{
			Name:            t.Name,
			Namespace:       t.Namespace,
			TargetContainer: t.TargetContainer,
			Source:          chaosDetails.ChaosPodName,
		}

		td.ContainerId, err = common.GetContainerID(td.Namespace, td.Name, td.TargetContainer, clients, td.Source)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container id")
		}

		// extract out the pid of the target container
		td.Pid, err = common.GetPID(experimentsDetails.ContainerRuntime, td.ContainerId, experimentsDetails.SocketPath, td.Source)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container pid")
		}

		td.CGroupManager, err = getCGroupManager(td)
		if err != nil {
			return stacktrace.Propagate(err, "could not get cgroup manager")
		}
		targets = append(targets, td)
	}

	// watching for the abort signal and revert the chaos if an abort signal is received
	go abortWatcher(targets, resultDetails.Name, chaosDetails.ChaosNamespace)

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(1)
	default:
	}

	done := make(chan error, 1)

	for index, t := range targets {
		targets[index].Cmd, err = injectChaos(t, stressors)
		if err != nil {
			return stacktrace.Propagate(err, "could not inject chaos")
		}
		log.Infof("successfully injected chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", t.Name); err != nil {
			if revertErr := terminateProcess(t); revertErr != nil {
				return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(revertErr).Error())}
			}
			return stacktrace.Propagate(err, "could not annotate chaosresult")
		}
	}

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	log.Info("[Wait]: Waiting for chaos completion")
	// channel to check the completion of the stress process
	go func() {
		var errList []string
		var exitErr error
		for _, t := range targets {
			if err := t.Cmd.Wait(); err != nil {
				if _, ok := err.(*exec.ExitError); ok {
					exitErr = err
					continue
				}
				errList = append(errList, err.Error())
			}
		}
		if exitErr != nil {
			done <- exitErr
		} else if len(errList) != 0 {
			done <- fmt.Errorf("err: %v", strings.Join(errList, ", "))
		} else {
			done <- nil
		}
	}()

	// check the timeout for the command
	// Note: timeout will occur when process didn't complete even after 10s of chaos duration
	timeout := time.After((time.Duration(experimentsDetails.ChaosDuration) + 30) * time.Second)

	select {
	case <-timeout:
		// the stress process gets timeout before completion
		log.Infof("[Chaos] The stress process is not yet completed after the chaos duration of %vs", experimentsDetails.ChaosDuration+30)
		log.Info("[Timeout]: Killing the stress process")
		var errList []string
		for _, t := range targets {
			if err = terminateProcess(t); err != nil {
				errList = append(errList, err.Error())
				continue
			}
			if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
				errList = append(errList, err.Error())
			}
		}
		if len(errList) != 0 {
			return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s]", strings.Join(errList, ","))}
		}
	case err := <-done:
		if err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if ok {
				status := exitErr.Sys().(syscall.WaitStatus)
				if status.Signaled() && status.Signal() == syscall.SIGKILL {
					// wait for the completion of abort handler
					time.Sleep(10 * time.Second)
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeExperimentAborted, Source: chaosDetails.ChaosPodName, Reason: fmt.Sprintf("process stopped with SIGTERM signal")}
				}
			}
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: chaosDetails.ChaosPodName, Reason: err.Error()}
		}
		log.Info("[Info]: Reverting Chaos")
		var errList []string
		for _, t := range targets {
			if err := terminateProcess(t); err != nil {
				errList = append(errList, err.Error())
				continue
			}
			log.Infof("successfully reverted chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
			if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
				errList = append(errList, err.Error())
			}
		}
		if len(errList) != 0 {
			return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s]", strings.Join(errList, ","))}
		}
	}

	return nil
}

//terminateProcess will remove the stress process from the target container after chaos completion
func terminateProcess(t targetDetails) error {
	if err := syscall.Kill(-t.Cmd.Process.Pid, syscall.SIGKILL); err != nil {
		if strings.Contains(err.Error(), ProcessAlreadyKilled) || strings.Contains(err.Error(), ProcessAlreadyFinished) {
			return nil
		}
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("failed to revert chaos: %s", err.Error())}
	}
	log.Infof("successfully reverted chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
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

//pidPath will get the pid path of the container
func pidPath(t targetDetails) cgroups.Path {
	processPath := "/proc/" + strconv.Itoa(t.Pid) + "/cgroup"
	paths, err := parseCgroupFile(processPath, t)
	if err != nil {
		return getErrorPath(errors.Wrapf(err, "parse cgroup file %s", processPath))
	}
	return getExistingPath(paths, t.Pid, "")
}

//parseCgroupFile will read and verify the cgroup file entry of a container
func parseCgroupFile(path string, t targetDetails) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("fail to parse cgroup: %s", err.Error())}
	}
	defer file.Close()
	return parseCgroupFromReader(file, t)
}

//parseCgroupFromReader will parse the cgroup file from the reader
func parseCgroupFromReader(r io.Reader, t targetDetails) (map[string]string, error) {
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
			return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("invalid cgroup entry: %q", text)}
		}
		for _, subs := range strings.Split(parts[1], ",") {
			if subs != "" {
				cgroups[subs] = parts[2]
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("buffer scanner failed: %s", err.Error())}
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
func findValidCgroup(path cgroups.Path, t targetDetails) (string, error) {
	for _, subsystem := range cgroupSubsystemList {
		path, err := path(cgroups.Name(subsystem))
		if err != nil {
			log.Errorf("fail to retrieve the cgroup path, subsystem: %v, target: %v, err: %v", subsystem, t.ContainerId, err)
			continue
		}
		if strings.Contains(path, t.ContainerId) {
			return path, nil
		}
	}
	return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: "could not find valid cgroup"}
}

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
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
func abortWatcher(targets []targetDetails, resultName, chaosNS string) {

	<-abort

	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("[Abort]: Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		for _, t := range targets {
			if err = terminateProcess(t); err != nil {
				log.Errorf("[Abort]: unable to revert for %v pod, err :%v", t.Name, err)
				continue
			}
			if err = result.AnnotateChaosResult(resultName, chaosNS, "reverted", "pod", t.Name); err != nil {
				log.Errorf("[Abort]: Unable to annotate the chaosresult for %v pod, err :%v", t.Name, err)
			}
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}

// getCGroupManager will return the cgroup for the given pid of the process
func getCGroupManager(t targetDetails) (interface{}, error) {
	if cgroups.Mode() == cgroups.Unified {
		groupPath, err := cgroupsv2.PidGroupPath(t.Pid)
		if err != nil {
			return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("fail to get pid group path: %s", err.Error())}
		}

		cgroup2, err := cgroupsv2.LoadManager("/sys/fs/cgroup", groupPath)
		if err != nil {
			return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("fail to load the cgroup: %s", err.Error())}
		}
		return cgroup2, nil
	}
	path := pidPath(t)
	cgroup, err := findValidCgroup(path, t)
	if err != nil {
		return nil, stacktrace.Propagate(err, "could not find valid cgroup")
	}
	cgroup1, err := cgroups.Load(cgroups.V1, cgroups.StaticPath(cgroup))
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("fail to load the cgroup: %s", err.Error())}
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

func injectChaos(t targetDetails, stressors string) (*exec.Cmd, error) {
	stressCommand := "pause nsutil -t " + strconv.Itoa(t.Pid) + " -p -- " + stressors
	log.Infof("[Info]: starting process: %v", stressCommand)

	// launch the stress-ng process on the target container in paused mode
	cmd := exec.Command("/bin/bash", "-c", stressCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	err = cmd.Start()
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("failed to start stress process: %s", err.Error())}
	}

	// add the stress process to the cgroup of target container
	if err = addProcessToCgroup(cmd.Process.Pid, t.CGroupManager); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("fail to add the stress process to cgroup %s and kill stress process: %s", err.Error(), killErr.Error())}
		}
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("fail to add the stress process to cgroup: %s", err.Error())}
	}

	log.Info("[Info]: Sending signal to resume the stress process")
	// wait for the process to start before sending the resume signal
	// TODO: need a dynamic way to check the start of the process
	time.Sleep(700 * time.Millisecond)

	// remove pause and resume or start the stress process
	if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("fail to remove pause and start the stress process: %s", err.Error())}
	}
	return cmd, nil
}

type targetDetails struct {
	Name            string
	Namespace       string
	TargetContainer string
	ContainerId     string
	Pid             int
	CGroupManager   interface{}
	Cmd             *exec.Cmd
	Source          string
}

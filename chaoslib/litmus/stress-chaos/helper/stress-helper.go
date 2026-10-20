package helper

import (
	"bufio"
	"bytes"
	"context"
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
	"github.com/palantir/stacktrace"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/stress-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
)

// list of cgroups in a container
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
func Helper(ctx context.Context, clients clients.ClientSets) {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "SimulatePodStressFault")
	defer span.End()

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

// prepareStressChaos contains the chaos preparation and injection steps
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

	var targets []*targetDetails

	for _, t := range targetList.Target {
		td := &targetDetails{
			Name:      t.Name,
			Namespace: t.Namespace,
			Source:    chaosDetails.ChaosPodName,
		}

		td.TargetContainers, err = common.GetTargetContainers(t.Name, t.Namespace, t.TargetContainer, chaosDetails.ChaosPodName, clients)
		if err != nil {
			return stacktrace.Propagate(err, "could not get target containers")
		}

		td.ContainerIds, err = common.GetContainerIDs(td.Namespace, td.Name, td.TargetContainers, clients, td.Source)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container ids")
		}

		for _, cid := range td.ContainerIds {
			// extract out the pid of the target container
			pid, err := common.GetPID(experimentsDetails.ContainerRuntime, cid, experimentsDetails.SocketPath, td.Source)
			if err != nil {
				return stacktrace.Propagate(err, "could not get container pid")
			}
			td.Pids = append(td.Pids, pid)
		}

		for i := range td.Pids {
			cGroupManagers, err, grpPath := getCGroupManager(td, i)
			if err != nil {
				return stacktrace.Propagate(err, "could not get cgroup manager")
			}
			td.GroupPath = grpPath
			td.CGroupManagers = append(td.CGroupManagers, cGroupManagers)
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":          td.Name,
			"Namespace":        td.Namespace,
			"TargetContainers": td.TargetContainers,
		})

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
		for i := range t.Pids {
			cmd, err := injectChaos(t, stressors, i, experimentsDetails.StressType)
			if err != nil {
				if revertErr := revertChaosForAllTargets(targets, resultDetails, chaosDetails.ChaosNamespace, index-1); revertErr != nil {
					return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(revertErr).Error())}
				}
				return stacktrace.Propagate(err, "could not inject chaos")
			}
			targets[index].Cmds = append(targets[index].Cmds, cmd)
			log.Infof("successfully injected chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainers[i])
		}

		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", t.Name); err != nil {
			if revertErr := revertChaosForAllTargets(targets, resultDetails, chaosDetails.ChaosNamespace, index); revertErr != nil {
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
			for i := range t.Cmds {
				if err := t.Cmds[i].Cmd.Wait(); err != nil {
					log.Infof("stress process failed, err: %v, out: %v", err, t.Cmds[i].Buffer.String())
					if _, ok := err.(*exec.ExitError); ok {
						exitErr = err
						continue
					}
					errList = append(errList, err.Error())
				}
			}
		}
		if exitErr != nil {
			oomKilled, err := checkOOMKilled(targets, clients, exitErr)
			if err != nil {
				log.Infof("could not check oomkilled, err: %v", err)
			}
			if !oomKilled {
				done <- exitErr
			}
			done <- nil
		} else if len(errList) != 0 {
			oomKilled, err := checkOOMKilled(targets, clients, fmt.Errorf("err: %v", strings.Join(errList, ", ")))
			if err != nil {
				log.Infof("could not check oomkilled, err: %v", err)
			}
			if !oomKilled {
				done <- fmt.Errorf("err: %v", strings.Join(errList, ", "))
			}
			done <- nil
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
		if err := revertChaosForAllTargets(targets, resultDetails, chaosDetails.ChaosNamespace, len(targets)-1); err != nil {
			return stacktrace.Propagate(err, "could not revert chaos")
		}
	case err := <-done:
		if err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if ok {
				status := exitErr.Sys().(syscall.WaitStatus)
				if status.Signaled() {
					log.Infof("process stopped with signal: %v", status.Signal())
				}
				if status.Signaled() && status.Signal() == syscall.SIGKILL {
					// wait for the completion of abort handler
					time.Sleep(10 * time.Second)
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeExperimentAborted, Source: chaosDetails.ChaosPodName, Reason: fmt.Sprintf("process stopped with SIGTERM signal")}
				}
			}
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: chaosDetails.ChaosPodName, Reason: err.Error()}
		}
		log.Info("[Info]: Reverting Chaos")
		if err := revertChaosForAllTargets(targets, resultDetails, chaosDetails.ChaosNamespace, len(targets)-1); err != nil {
			return stacktrace.Propagate(err, "could not revert chaos")
		}
	}

	return nil
}

func revertChaosForAllTargets(targets []*targetDetails, resultDetails *types.ResultDetails, chaosNs string, index int) error {
	var errList []string
	for i := 0; i <= index; i++ {
		if err := terminateProcess(targets[i]); err != nil {
			errList = append(errList, err.Error())
			continue
		}
		if err := result.AnnotateChaosResult(resultDetails.Name, chaosNs, "reverted", "pod", targets[i].Name); err != nil {
			errList = append(errList, err.Error())
		}
	}

	if len(errList) != 0 {
		return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s]", strings.Join(errList, ","))}
	}
	return nil
}

// checkOOMKilled checks if the container within the target pods failed due to an OOMKilled error.
func checkOOMKilled(targets []*targetDetails, clients clients.ClientSets, chaosError error) (bool, error) {
	// Check each container in the pod
	for i := 0; i < 3; i++ {
		for _, t := range targets {
			// Fetch the target pod
			targetPod, err := clients.KubeClient.CoreV1().Pods(t.Namespace).Get(context.Background(), t.Name, v1.GetOptions{})
			if err != nil {
				return false, cerrors.Error{
					ErrorCode: cerrors.ErrorTypeStatusChecks,
					Target:    fmt.Sprintf("{podName: %s, namespace: %s}", t.Name, t.Namespace),
					Reason:    err.Error(),
				}
			}
			for _, c := range targetPod.Status.ContainerStatuses {
				if utils.Contains(c.Name, t.TargetContainers) {
					// Check for OOMKilled and restart
					if c.LastTerminationState.Terminated != nil && c.LastTerminationState.Terminated.ExitCode == 137 {
						log.Warnf("[Warning]: The target container '%s' of pod '%s' got OOM Killed, err: %v", c.Name, t.Name, chaosError)
						return true, nil
					}
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
	return false, nil
}

// terminateProcess will remove the stress process from the target container after chaos completion
func terminateProcess(t *targetDetails) error {
	var errList []string
	for i := range t.Cmds {
		if t.Cmds[i] != nil && t.Cmds[i].Cmd.Process != nil {
			if err := syscall.Kill(-t.Cmds[i].Cmd.Process.Pid, syscall.SIGKILL); err != nil {
				if strings.Contains(err.Error(), ProcessAlreadyKilled) || strings.Contains(err.Error(), ProcessAlreadyFinished) {
					continue
				}
				errList = append(errList, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[i]), Reason: fmt.Sprintf("failed to revert chaos: %s", err.Error())}.Error())
				continue
			}
			log.Infof("successfully reverted chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainers[i])
		}
	}
	if len(errList) != 0 {
		return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s]", strings.Join(errList, ","))}
	}
	return nil
}

// prepareStressor will set the required stressors for the given experiment
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
		log.Fatalf("stressor for %v experiment is not supported", experimentDetails.ExperimentName)
	}
	return stressArgs
}

// pidPath will get the pid path of the container
func pidPath(t *targetDetails, index int) cgroups.Path {
	processPath := "/proc/" + strconv.Itoa(t.Pids[index]) + "/cgroup"
	paths, err := parseCgroupFile(processPath, t, index)
	if err != nil {
		return getErrorPath(errors.Wrapf(err, "parse cgroup file %s", processPath))
	}
	return getExistingPath(paths, t.Pids[index], "")
}

// parseCgroupFile will read and verify the cgroup file entry of a container
func parseCgroupFile(path string, t *targetDetails, index int) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: fmt.Sprintf("fail to parse cgroup: %s", err.Error())}
	}
	defer file.Close()
	return parseCgroupFromReader(file, t, index)
}

// parseCgroupFromReader will parse the cgroup file from the reader
func parseCgroupFromReader(r io.Reader, t *targetDetails, index int) (map[string]string, error) {
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
			return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: fmt.Sprintf("invalid cgroup entry: %q", text)}
		}
		for _, subs := range strings.Split(parts[1], ",") {
			if subs != "" {
				cgroups[subs] = parts[2]
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: fmt.Sprintf("buffer scanner failed: %s", err.Error())}
	}

	return cgroups, nil
}

// getExistingPath will be used to get the existing valid cgroup path
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

// getErrorPath will give the invalid cgroup path
func getErrorPath(err error) cgroups.Path {
	return func(_ cgroups.Name) (string, error) {
		return "", err
	}
}

// getCgroupDestination will validate the subsystem with the mountpath in container mountinfo file.
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

// findValidCgroup will be used to get a valid cgroup path
func findValidCgroup(path cgroups.Path, t *targetDetails, index int) (string, error) {
	for _, subsystem := range cgroupSubsystemList {
		path, err := path(cgroups.Name(subsystem))
		if err != nil {
			log.Errorf("fail to retrieve the cgroup path, subsystem: %v, target: %v, err: %v", subsystem, t.ContainerIds[index], err)
			continue
		}
		if strings.Contains(path, t.ContainerIds[index]) {
			return path, nil
		}
	}
	return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: "could not find valid cgroup"}
}

// getENV fetches all the env variables from the runner pod
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
func abortWatcher(targets []*targetDetails, resultName, chaosNS string) {

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
func getCGroupManager(t *targetDetails, index int) (interface{}, error, string) {
	if cgroups.Mode() == cgroups.Unified {
		groupPath := ""
		output, err := exec.Command("bash", "-c", fmt.Sprintf("nsenter -t 1 -C -m -- cat /proc/%v/cgroup", t.Pids[index])).CombinedOutput()
		if err != nil {
			return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: fmt.Sprintf("fail to get the cgroup: %s :%v", err.Error(), output)}, ""
		}
		log.Infof("cgroup output: %s", string(output))
		parts := strings.Split(string(output), ":")
		if len(parts) < 3 {
			return "", fmt.Errorf("invalid cgroup entry: %s", string(output)), ""
		}
		if strings.HasSuffix(parts[len(parts)-3], "0") && parts[len(parts)-2] == "" {
			groupPath = parts[len(parts)-1]
		}
		log.Infof("group path: %s", groupPath)

		cgroup2, err := cgroupsv2.LoadManager("/sys/fs/cgroup", string(groupPath))
		if err != nil {
			return nil, errors.Errorf("Error loading cgroup v2 manager, %v", err), ""
		}
		return cgroup2, nil, groupPath
	}
	path := pidPath(t, index)
	cgroup, err := findValidCgroup(path, t, index)
	if err != nil {
		return nil, stacktrace.Propagate(err, "could not find valid cgroup"), ""
	}
	cgroup1, err := cgroups.Load(cgroups.V1, cgroups.StaticPath(cgroup))
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: fmt.Sprintf("fail to load the cgroup: %s", err.Error())}, ""
	}

	return cgroup1, nil, ""
}

// addProcessToCgroup will add the process to cgroup
// By default it will add to v1 cgroup
func addProcessToCgroup(pid int, control interface{}, groupPath string) error {
	if cgroups.Mode() == cgroups.Unified {
		args := []string{"-t", "1", "-C", "--", "sudo", "sh", "-c", fmt.Sprintf("echo %d >> /sys/fs/cgroup%s/cgroup.procs", pid, strings.ReplaceAll(groupPath, "\n", ""))}
		output, err := exec.Command("nsenter", args...).CombinedOutput()
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosInject,
				Reason:    fmt.Sprintf("failed to add process to cgroup %s: %v", string(output), err),
			}
		}
		return nil
	}
	var cgroup1 = control.(cgroups.Cgroup)
	return cgroup1.Add(cgroups.Process{Pid: pid})
}

func injectChaos(t *targetDetails, stressors string, index int, stressType string) (*Command, error) {
	stressCommand := fmt.Sprintf("pause nsutil -t %v -p -- %v", strconv.Itoa(t.Pids[index]), stressors)
	// for io stress,we need to enter into mount ns of the target container
	// enabling it by passing -m flag
	if stressType == "pod-io-stress" {
		stressCommand = fmt.Sprintf("pause nsutil -t %v -p -m -- %v", strconv.Itoa(t.Pids[index]), stressors)
	}
	log.Infof("[Info]: starting process: %v", stressCommand)

	// launch the stress-ng process on the target container in paused mode
	cmd := exec.Command("/bin/bash", "-c", stressCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Start()
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: fmt.Sprintf("failed to start stress process: %s", err.Error())}
	}

	// add the stress process to the cgroup of target container
	if err = addProcessToCgroup(cmd.Process.Pid, t.CGroupManagers[index], t.GroupPath); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: fmt.Sprintf("fail to add the stress process to cgroup %s and kill stress process: %s", err.Error(), killErr.Error())}
		}
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: fmt.Sprintf("fail to add the stress process to cgroup: %s", err.Error())}
	}

	log.Info("[Info]: Sending signal to resume the stress process")
	// wait for the process to start before sending the resume signal
	// TODO: need a dynamic way to check the start of the process
	time.Sleep(700 * time.Millisecond)

	// remove pause and resume or start the stress process
	if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainers[index]), Reason: fmt.Sprintf("fail to remove pause and start the stress process: %s", err.Error())}
	}
	return &Command{
		Cmd:    cmd,
		Buffer: buf,
	}, nil
}

type targetDetails struct {
	Name             string
	Namespace        string
	TargetContainers []string
	ContainerIds     []string
	Pids             []int
	CGroupManagers   []interface{}
	Cmds             []*Command
	Source           string
	GroupPath        string
}

type Command struct {
	Cmd    *exec.Cmd
	Buffer bytes.Buffer
}

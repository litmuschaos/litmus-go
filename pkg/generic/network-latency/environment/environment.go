package environment

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"

	types "github.com/litmuschaos/litmus-go/pkg/generic/network-latency/types"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *types.ExperimentDetails, expName string) {
	experimentDetails.ExperimentName = expName
	experimentDetails.ChaosNamespace = os.Getenv("CHAOS_NAMESPACE")
	experimentDetails.EngineName = os.Getenv("CHAOSENGINE")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(os.Getenv("TOTAL_CHAOS_DURATION"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(os.Getenv("CHAOS_INTERVAL"))
	experimentDetails.RampTime, _ = strconv.Atoi(os.Getenv("RAMP_TIME"))
	experimentDetails.ChaosLib = os.Getenv("LIB")
	experimentDetails.ChaosServiceAccount = os.Getenv("CHAOS_SERVICE_ACCOUNT")
	experimentDetails.AppNS = os.Getenv("APP_NAMESPACE")
	experimentDetails.AppLabel = os.Getenv("APP_LABEL")
	experimentDetails.AppKind = os.Getenv("APP_KIND")
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.AuxiliaryAppInfo = os.Getenv("AUXILIARY_APPINFO")
	experimentDetails.InstanceID = os.Getenv("INSTANCE_ID")
	experimentDetails.ChaosPodName = os.Getenv("POD_NAME")
	experimentDetails.Latency, _ = strconv.ParseFloat(os.Getenv("LATENCY"), 32)
	experimentDetails.Jitter, _ = strconv.ParseFloat(os.Getenv("JITTER"), 32)
	experimentDetails.ChaosNode = os.Getenv("CHAOS_NODE")
	experimentDetails.Delay, _ = strconv.Atoi(Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(Getenv("STATUS_CHECK_TIMEOUT", "180"))
}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}

//Resolver lookups the ips of given hostnames from the dependencies
//For now, it doesn't make use of the port field as we need to change the tc commands to include that in the match filter
func Resolver(config types.Config) (types.ConfigTC, error) {

	deps := config.Dependencies
	var conf types.ConfigTC

	for _, dep := range deps {

		ips, err := net.LookupIP(dep.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not get IPs: %v\n", err)
			return types.ConfigTC{}, err
		}
		for _, ip := range ips {
			fmt.Printf("%s\n IN A %s\n", dep.Name, ip.String())
			conf.IP = append(conf.IP, ip)
			conf.Port = append(conf.Port, dep.Port)
		}
	}
	fmt.Printf("%v\n", conf)
	return conf, nil
}

//Dependencies function finds the dependencies of the mounted yaml: name and port
func Dependencies() (types.Config, error) {

	filename := "/mnt/dependencies.yaml"
	c, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal().Err(err).Msg(fmt.Sprintf("Error reading file %s", filename))
		return types.Config{}, err
	}

	exp := types.Definition{}
	err = yaml.Unmarshal(c, &exp)
	if err != nil {
		return types.Config{}, err
	}

	fmt.Printf("%v\n", exp.Experiment.Config)

	return exp.Experiment.Config, nil

}

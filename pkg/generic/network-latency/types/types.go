package types

import (
	clientTypes "k8s.io/apimachinery/pkg/types"
	"net"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName      string
	EngineName          string
	ChaosDuration       int
	ChaosInterval       int
	RampTime            int
	ChaosLib            string
	ChaosServiceAccount string
	AppNS               string
	AppLabel            string
	AppKind             string
	ChaosUID            clientTypes.UID
	AuxiliaryAppInfo    string
	InstanceID          string
	ChaosNamespace      string
	ChaosPodName        string
	Latency             float64
	Jitter              float64
	ChaosNode           string
	Timeout             int
	Delay               int
}

// Definition is the chaos experiment definition coming from a user input file.
type Definition struct {
	Experiment Experiment
}

// Experiment defines the type and target of the experiment.
type Experiment struct {
	Config Config
}

// Config for a specific experiment action
type Config struct {
	Dependencies []Dependency
}

// Dependency defines name  and port
type Dependency struct {
	Name string
	Port int
}

// ConfigTC defines IP  and port
type ConfigTC struct {
	IP   []net.IP
	Port []int
}

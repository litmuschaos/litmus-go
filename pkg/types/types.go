package types

const (
	PreChaosCheck                   string = "PreChaosCheck"
	PostChaosCheck                  string = "PostChaosCheck"
	Summary                         string = "Summary"
	ChaosInject                     string = "ChaosInject"
)

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName      string
	EngineName          string
	ChaosDuration       int
	ChaosInterval       int
	RampTime            int
	Force               bool
	ChaosLib            string
	ChaosServiceAccount string
	AppNS               string
	AppLabel            string
	AppKind             string
	KillCount           int
	ChaosUID            string
	AuxiliaryAppInfo    string
	InstanceID          string
	ChaosNamespace      string
	ChaosPodName        string
}

// ResultDetails is for collecting all the chaos-result-related details
type ResultDetails struct {
	Name                string
	Verdict             string
	FailStep            string
	Phase               string
}

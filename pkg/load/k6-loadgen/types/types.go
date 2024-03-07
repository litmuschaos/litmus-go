package types

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName     string
	EngineName         string
	ChaosDuration      int
	ChaosInterval      int
	RampTime           int
	AppNS              string
	AppLabel           string
	AppKind            string
	ChaosNamespace     string
	Timeout            int
	Delay              int
	PodsAffectedPerc   int
	LIBImagePullPolicy string
	LIBImage           string
	ScriptSecretName   string
	ScriptSecretKey    string
}

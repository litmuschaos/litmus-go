package types

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName     string
	EngineName         string
	ChaosDuration      int
	RampTime           int
	ChaosNamespace     string
	Timeout            int
	Delay              int
	LIBImagePullPolicy string
	LIBImage           string
	ScriptSecretName   string
	ScriptSecretKey    string
	OTELMetricPrefix   string
}

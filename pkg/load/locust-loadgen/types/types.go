package types

// ExperimentDetails is for collecting all the experiment-related details
type ExperimentDetails struct {
	ExperimentName     string
	ChaosNamespace     string
	EngineName         string
	ChaosDuration      int
	RampTime           int
	Delay              int
	Timeout            int
	LIBImage           string
	LIBImagePullPolicy string
	Host               string
	ConfigMapName      string
	ConfigMapKey       string
	Users              int
	SpawnRate          int
}

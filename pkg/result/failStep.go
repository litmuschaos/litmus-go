package result

const (
	ResultUpdatePreChaos      = "[pre-chaos]: failed to create/update chaos result"
	AUTStatusCheckPreChaos    = "[pre-chaos]: failed in AUT status checks"
	ProbeExecutionPreChaos    = "[pre-chaos]: failed in probe execution"
	AUTStatusCheckPostChaos   = "[post-chaos]: failed in AUT status checks"
	ProbeExecutionPostChaos   = "[post-chaos]: failed in probe execution"
	InsufficientTargetDetails = "[chaos]: insufficient target application details"
	TargetPodList             = "[chaos]: failed to get the target pod list"
	TargetPodListByNodeLabel  = "[chaos]: failed to get the target pod list by node labels"
	TargetPodByTargetPodENV   = "[chaos]: failed to get the target pod list by TARGET_PODS env"
	ServiceAccount            = "[chaos]: failed to get the service account name"
	HelperData                = "[chaos]: failed to inherit experiment pod properties for helper pod"
	ProbeExecutionDuringChaos = "[chaos]: failed while running onchaos probes"
	TargetContainer           = "[chaos]: failed to get target container"
	CreateHelperPod           = "[chaos]: failed to launch helper pod(s)"
	CheckHelperStatus         = "[chaos]: unhealthy helper pod(s)"
	HelperPodFailed           = "[chaos]: helper pod(s) failed, check helper pod logs"
	HelperDelete              = "[chaos]: failed to delete helper pod(s)"
)

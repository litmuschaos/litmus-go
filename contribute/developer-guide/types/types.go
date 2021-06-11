package types

const (
	IconPath string = "/contribute/developer-guide/icons/k8s.png"
)

// Experiment contains details of all the attributes, which is needed for the new experiments generations
type Experiment struct {
	//Name is the name of new chaos experiment
	Name string `json:"name"`
	// Version is the version of new chaos experiment
	// it should be 0.1.0
	Version string `json:"version"`
	// Category define the category of the new chaos experiment
	// for example, we have generic, kafka, cassandra, OpenEBS categories
	Category string `json:"category"`
	// It should contain the link of the new chaos experiment
	// this link will be in the form:  https://github.com/litmuschaos/litmus-go/tree/master/<category>/<experiment-name>
	// this value is needed for csv file(to display in the chaoshub)
	Repository string `json:"repository"`
	// It contains the link of the litmus slack
	Community string `json:"community"`
	// It contains the detailed description of new chaos experiment
	Description string `json:"description"`
	// It contains the keywords or hashtag for the chaos experiment
	// which can better define that chaos experiment
	Keywords []string `json:"keywords"`
	// It contains the supported platforms for the chaos experiment
	Platforms []string `json:"platforms"`
	// Scope define the scope of chaos experiment
	// whether it is namespaced or cluster scoped
	Scope string `json:"scope"`
	// This check define whether you want auxiliary status check or not
	// while generating new chaos experiment
	AuxiliaryAppCheck bool `json:"auxiliaryappcheck,omitempty"`
	// Permissions contains the list of all permission needed inside cluster-role to execute the experiment
	// it contains list of all apigroups, resources and verbs
	Permissions []Permission `json:"permissions"`
	// it define the maturity level of the chaos experiment
	// whether it is alpha or beta
	Maturity string `json:"maturity"`
	// it contains list of all the maintainers of the new chaos experiment
	Maintainers []MaintainerDetails `json:"maintainers"`
	// it contains the name of provider, the company or individual name
	Provider ProviderDetails `json:"provider"`
	// it contains the minimum kubernetes version, which supports this experiment
	MinKubernetesVersion string `json:"minkubernetesversion"`
	// reference contains the all the references like docs, youtube video link, etc
	References []ReferencesDetails `json:"references"`
}

// Permission contains the list of all permission needed inside cluster-role to execute the experiment
// it contains list of all apigroups, resources and verbs
type Permission struct {
	// ApiGroups contains list of all the apiGroup needed for the experiment execution
	APIGroups []string `json:"apigroups,omitempty"`
	// resources contains list of all the resources needed for the experiment execution
	Resources []string `json:"resources"`
	// verbs contains list of all the verbs(CRUD operations) needed for the experiment execution
	Verbs []string `json:"verbs"`
}

// ProviderDetails contains the name of provider, the company or individual name
type ProviderDetails struct {
	// it contains the name of provider
	Name string `json:"name"`
}

// MaintainerDetails contains details of all the maintainers of the new chaos experiment
type MaintainerDetails struct {
	// it contains the name of maintainer
	Name string `json:"name"`
	// it contain the email of maintainer
	Email string `json:"email"`
}

// ReferencesDetails contains details of all the references like docs, youtube video link, etc
type ReferencesDetails struct {
	// it contains the name of reference
	Name string `json:"name"`
	// it contain the url of reference
	URL string `json:"url"`
}

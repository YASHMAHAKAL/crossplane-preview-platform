package preview

type Config struct {
	Service           string          `json:"service"`
	Repository        string          `json:"repository"`
	TrustedAuthors    []string        `json:"trustedAuthors"`
	Sizes             map[string]Size `json:"sizes"`
	DefaultSize       string          `json:"defaultSize"`
	DefaultTTLMinutes int             `json:"defaultTTLMinutes"`
	MaxTTLMinutes     int             `json:"maxTTLMinutes"`
	MaxActivePreviews int             `json:"maxActivePreviews"`
	AllowedCRD        AllowedCRD      `json:"allowedCRD"`
}

// PreviewDefaults keeps existing operator configs usable while the request
// template is retired. Defaults come from trusted local config, never a PR.
func (config Config) PreviewDefaults() Request {
	size := config.DefaultSize
	if size == "" {
		size = "small"
	}
	ttl := config.DefaultTTLMinutes
	if ttl == 0 {
		ttl = 120
	}
	return Request{Size: size, TTLMinutes: ttl}
}

type Size struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type AllowedCRD struct {
	Name           string `json:"name"`
	Group          string `json:"group"`
	Kind           string `json:"kind"`
	ManifestSHA256 string `json:"manifestSHA256"`
}

type PullRequest struct {
	Number     int    `json:"number"`
	HeadSHA    string `json:"headSHA"`
	Repository string `json:"repository"`
	Author     string `json:"author"`
	Fork       bool   `json:"fork"`
	State      string `json:"state"`
}

type Request struct {
	Size       string `json:"size"`
	TTLMinutes int    `json:"ttlMinutes"`
}

type CI struct {
	State       string `json:"state"`
	HeadSHA     string `json:"headSHA"`
	ImageDigest string `json:"imageDigest"`
}

type ChangedFile struct {
	Path    string `json:"path"`
	Status  string `json:"status,omitempty"`
	Content string `json:"content,omitempty"`
}

type ResourceAmount struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type DeploymentResources struct {
	Requests ResourceAmount `json:"requests"`
	Limits   ResourceAmount `json:"limits"`
}

type DeploymentSpec struct {
	Replicas  int                 `json:"replicas"`
	Resources DeploymentResources `json:"resources"`
}

// IncidentPolicySpec is the only PR-controlled cluster API setting allowed
// into trusted GitOps. The Function reconstructs the CRD from this value.
type IncidentPolicySpec struct {
	AllowCritical bool `json:"allowCritical"`
}

type Snapshot struct {
	PR             PullRequest   `json:"pr"`
	Request        Request       `json:"request"`
	CI             CI            `json:"ci"`
	Candidate      CandidateCI   `json:"candidate,omitempty"`
	ActivePreviews int           `json:"activePreviews"`
	Files          []ChangedFile `json:"files"`
}

type CandidateCI struct {
	State         string `json:"state,omitempty"`
	HeadSHA       string `json:"headSHA,omitempty"`
	PackageDigest string `json:"packageDigest,omitempty"`
	WorkflowRunID int64  `json:"workflowRunID,omitempty"`
}

type Decision struct {
	PolicyVersion  string              `json:"policyVersion"`
	Phase          string              `json:"phase"`
	Mode           string              `json:"mode,omitempty"`
	ReasonCodes    []string            `json:"reasonCodes"`
	Evidence       []string            `json:"evidence"`
	HeadSHA        string              `json:"headSHA"`
	Capabilities   []string            `json:"capabilities,omitempty"`
	ImageDigest    string              `json:"imageDigest,omitempty"`
	Candidate      *CandidateCI        `json:"candidate,omitempty"`
	Request        *Request            `json:"request,omitempty"`
	Deployment     *DeploymentSpec     `json:"deployment,omitempty"`
	IncidentPolicy *IncidentPolicySpec `json:"incidentPolicy,omitempty"`
}

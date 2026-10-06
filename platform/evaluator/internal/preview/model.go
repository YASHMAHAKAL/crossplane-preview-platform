package preview

type Config struct {
	Service           string          `json:"service"`
	Repository        string          `json:"repository"`
	TrustedAuthors    []string        `json:"trustedAuthors"`
	Sizes             map[string]Size `json:"sizes"`
	MaxTTLMinutes     int             `json:"maxTTLMinutes"`
	MaxActivePreviews int             `json:"maxActivePreviews"`
	AllowedCRD        AllowedCRD      `json:"allowedCRD"`
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
	Content string `json:"content,omitempty"`
}

type Snapshot struct {
	PR             PullRequest   `json:"pr"`
	Request        Request       `json:"request"`
	CI             CI            `json:"ci"`
	ActivePreviews int           `json:"activePreviews"`
	Files          []ChangedFile `json:"files"`
}

type Decision struct {
	PolicyVersion string   `json:"policyVersion"`
	Phase         string   `json:"phase"`
	Mode          string   `json:"mode,omitempty"`
	ReasonCodes   []string `json:"reasonCodes"`
	Evidence      []string `json:"evidence"`
	HeadSHA       string   `json:"headSHA"`
	Capabilities  []string `json:"capabilities,omitempty"`
	ImageDigest   string   `json:"imageDigest,omitempty"`
	Request       *Request `json:"request,omitempty"`
}

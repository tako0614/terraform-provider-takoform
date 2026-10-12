// Package hostv2 implements the isolated Takoform Host API v2 HTTP lane.
// It has no compatibility or fallback behavior for the retained v1 clients.
package hostv2

const (
	APIVersion       = "forms.takoform.com/v2"
	DiscoveryPath    = "/.well-known/takoform/v2"
	maxResponseBytes = 8 * 1024 * 1024
)

type Discovery struct {
	API            string         `json:"api"`
	BaseURL        string         `json:"baseUrl"`
	Documentation  string         `json:"documentation"`
	Authentication Authentication `json:"authentication"`
	Capabilities   Capabilities   `json:"capabilities"`
	Limits         Limits         `json:"limits"`
}

type Authentication struct {
	Schemes       []string `json:"schemes"`
	Documentation string   `json:"documentation"`
}

type Capabilities struct {
	Offerings     bool `json:"offerings"`
	Previews      bool `json:"previews"`
	PrivateInputs bool `json:"privateInputs"`
}

type Limits struct {
	MaxRequestBytes     int64 `json:"maxRequestBytes"`
	MaxPageSize         int64 `json:"maxPageSize"`
	ReplayWindowSeconds int64 `json:"replayWindowSeconds"`
}

type Support struct {
	Form          string   `json:"form"`
	Supported     bool     `json:"supported"`
	Operations    []string `json:"operations"`
	PrivateInputs bool     `json:"privateInputs"`
}

type Offering struct {
	ID          string `json:"id"`
	Revision    string `json:"revision"`
	Form        string `json:"form"`
	Label       string `json:"label"`
	Description string `json:"description"`
	TermsURL    string `json:"termsUrl,omitempty"`
}

type OfferingSelection struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

type PageOptions struct {
	Limit  int64
	Cursor string
}

type ResourceFilters struct {
	Space string
	Name  string
	Form  string
}

type Resource struct {
	UID                string             `json:"uid"`
	Form               string             `json:"form"`
	Space              string             `json:"space"`
	Name               string             `json:"name"`
	Offering           *OfferingSelection `json:"offering,omitempty"`
	Generation         int64              `json:"generation"`
	ObservedGeneration int64              `json:"observedGeneration"`
	ObservedAt         *string            `json:"observedAt"`
	Phase              string             `json:"phase"`
	Spec               map[string]any     `json:"spec"`
	Observed           map[string]any     `json:"observed"`
	Output             map[string]any     `json:"output"`
	LastOperation      string             `json:"lastOperation"`
}

type CreateRequest struct {
	Form          string             `json:"form"`
	Space         string             `json:"space"`
	Name          string             `json:"name"`
	Offering      *OfferingSelection `json:"offering,omitempty"`
	Spec          map[string]any     `json:"spec"`
	PrivateInputs *map[string]string `json:"privateInputs,omitempty"`
}

type UpdateRequest struct {
	Spec          map[string]any     `json:"spec"`
	PrivateInputs *map[string]string `json:"privateInputs,omitempty"`
}

type Operation struct {
	ID            string          `json:"id"`
	ResourceUID   string          `json:"resourceUid"`
	Action        string          `json:"action"`
	Generation    int64           `json:"generation"`
	Status        string          `json:"status"`
	Effect        string          `json:"effect"`
	CreatedAt     string          `json:"createdAt"`
	UpdatedAt     string          `json:"updatedAt"`
	RetainUntil   string          `json:"retainUntil"`
	Error         *OperationError `json:"error,omitempty"`
	InputRequired *InputRequired  `json:"inputRequired,omitempty"`
}

type OperationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InputRequired struct {
	Names  []string `json:"names"`
	Reason string   `json:"reason"`
}

// IndeterminateError means a mutation may have been accepted. The caller must
// reconcile by reading the Resource/Operation and must not create a new intent.
type IndeterminateError struct {
	Action string
}

func (e *IndeterminateError) Error() string {
	return "takoform v2: " + e.Action + " outcome is indeterminate; reconcile the original intent"
}

// APIError represents a bounded, parsed Problem Details response. Raw bodies
// and server-provided detail text are intentionally not exposed to callers.
type APIError struct {
	Status     int
	Code       string
	RetryAfter string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return "takoform v2: host returned HTTP error"
	}
	return "takoform v2: host returned HTTP error " + e.Code
}

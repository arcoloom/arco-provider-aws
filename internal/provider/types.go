package provider

import "time"

type Cloud string

const (
	CloudAWS Cloud = "aws"
)

const (
	AuthMethodAWSDefaultCredentials = "default_credentials"
	AuthMethodAWSStaticAccessKey    = "static_access_key"
	AuthMethodAWSAssumeRole         = "assume_role"
)

type ResourcePlane string

const (
	ResourcePlaneCompute ResourcePlane = "compute"
	ResourcePlaneStorage ResourcePlane = "storage"
	ResourcePlaneNetwork ResourcePlane = "network"
)

type SchemaAttributeType string

const (
	SchemaAttributeTypeString     SchemaAttributeType = "string"
	SchemaAttributeTypeBool       SchemaAttributeType = "bool"
	SchemaAttributeTypeInt64      SchemaAttributeType = "int64"
	SchemaAttributeTypeFloat64    SchemaAttributeType = "float64"
	SchemaAttributeTypeStringList SchemaAttributeType = "string_list"
	SchemaAttributeTypeStringMap  SchemaAttributeType = "string_map"
)

type SchemaAttribute struct {
	Name         string
	Type         SchemaAttributeType
	Required     bool
	Optional     bool
	Computed     bool
	Sensitive    bool
	Description  string
	DefaultValue any
}

type ResourceSchema struct {
	Type        string
	Description string
	Attributes  []SchemaAttribute
}

type AuthMethod struct {
	Name        string
	DisplayName string
	Description string
	Fields      []SchemaAttribute
}

type Metadata struct {
	Name              string
	Version           string
	Cloud             string
	AuthMethods       []AuthMethod
	SupportedServices []string
	Capabilities      map[string]string
	ResourcePlanes    []ResourcePlane
}

type RequestContext struct {
	RequestID string
	Caller    string
	TraceID   string
}

type ConnectionScope struct {
	ScopeID        string
	Region         string
	Endpoint       string
	Attributes     map[string]string
	EndpointRegion string
}

type AWSCredentials struct {
	UseDefaultCredentialsChain bool
	Profile                    string
	AccessKeyID                string
	SecretAccessKey            string
	SessionToken               string
	RoleARN                    string
	ExternalID                 string
	RoleSessionName            string
	SourceIdentity             string
}

type AWSAccount struct {
	Name        string
	Credentials AWSCredentials
}

type Credentials struct {
	AWS         *AWSCredentials
	AWSAccounts []AWSAccount
}

type ValidateConnectionRequest struct {
	Context     RequestContext
	Credentials Credentials
	Scope       ConnectionScope
	Options     map[string]string
}

type Warning struct {
	Code    string
	Message string
}

type LaunchFailureClass string

const (
	LaunchFailureClassCapacity LaunchFailureClass = "capacity"
	LaunchFailureClassQuota    LaunchFailureClass = "quota"
	LaunchFailureClassPrice    LaunchFailureClass = "price"
	LaunchFailureClassAPI      LaunchFailureClass = "api"
	LaunchFailureClassAuth     LaunchFailureClass = "auth"
	LaunchFailureClassConfig   LaunchFailureClass = "config"
	LaunchFailureClassProvider LaunchFailureClass = "provider"
)

type LaunchFailureScope string

const (
	LaunchFailureScopePlacement LaunchFailureScope = "placement"
	LaunchFailureScopeZone      LaunchFailureScope = "zone"
	LaunchFailureScopeRegion    LaunchFailureScope = "region"
	LaunchFailureScopeAccount   LaunchFailureScope = "account"
	LaunchFailureScopeProvider  LaunchFailureScope = "provider"
	LaunchFailureScopeJob       LaunchFailureScope = "job"
)

type LaunchFailure struct {
	Code       string
	Class      LaunchFailureClass
	Scope      LaunchFailureScope
	Retryable  bool
	Message    string
	RawCode    string
	Attributes map[string]string
}

type ValidateConnectionResult struct {
	Accepted bool
	Message  string
	Warnings []Warning
}

type PingResult struct {
	Payload   string
	Timestamp time.Time
}

type ListRegionsRequest struct {
	Context     RequestContext
	Credentials Credentials
	Scope       ConnectionScope
	Options     map[string]string
}

type ListRegionsResult struct {
	Items    []Region
	Warnings []Warning
}

type AvailabilityZone struct {
	Name               string
	ZoneID             string
	Region             string
	State              string
	ZoneType           string
	GroupName          string
	NetworkBorderGroup string
	ParentZoneID       string
	ParentZoneName     string
	OptInStatus        string
}

type ListAvailabilityZonesRequest struct {
	Context           RequestContext
	Credentials       Credentials
	Scope             ConnectionScope
	Region            string
	AvailabilityZones []string
	Options           map[string]string
}

type ListAvailabilityZonesResult struct {
	Items    []AvailabilityZone
	Warnings []Warning
}

type WatchMarketFeedRequest struct {
	Context     RequestContext
	Credentials Credentials
	Scope       ConnectionScope
	ResumeToken string
	Options     map[string]string
}

type MarketOffering struct {
	ScopeID          string
	Region           string
	AvailabilityZone string
	ZoneID           string
	InstanceType     string
	PurchaseOption   PurchaseOption
	CPUMilli         int32
	MemoryMiB        int32
	GPUCount         int32
	HourlyPriceUSD   float64
	Attributes       map[string]string
}

type WatchMarketFeedEventType string

const (
	WatchMarketFeedEventTypeBegin     WatchMarketFeedEventType = "begin"
	WatchMarketFeedEventTypeChunk     WatchMarketFeedEventType = "chunk"
	WatchMarketFeedEventTypeCommit    WatchMarketFeedEventType = "commit"
	WatchMarketFeedEventTypeHeartbeat WatchMarketFeedEventType = "heartbeat"
	WatchMarketFeedEventTypeWarning   WatchMarketFeedEventType = "warning"
)

type WatchMarketFeedEvent struct {
	Type          WatchMarketFeedEventType
	SnapshotToken string
	ResumeToken   string
	Offerings     []MarketOffering
	Warnings      []Warning
}

type GetSpotDataRequest struct {
	Context           RequestContext
	Credentials       Credentials
	Scope             ConnectionScope
	InstanceTypes     []string
	Region            string
	AvailabilityZones []string
	Options           map[string]string
}

type SpotInventory struct {
	Offered          bool
	Status           string
	HasCapacityScore bool
	CapacityScore    int32
}

type SpotData struct {
	InstanceType     string
	Region           string
	AvailabilityZone string
	HasPrice         bool
	Price            string
	Currency         string
	Timestamp        time.Time
	Inventory        SpotInventory
}

type GetSpotDataResult struct {
	Items    []SpotData
	Warnings []Warning
}

type InstanceMarketType string

const (
	InstanceMarketTypeOnDemand InstanceMarketType = "on-demand"
	InstanceMarketTypeSpot     InstanceMarketType = "spot"
)

type InstanceTag struct {
	Key   string
	Value string
}

type StartInstanceRequest struct {
	Context          RequestContext
	Credentials      Credentials
	Scope            ConnectionScope
	StackName        string
	InstanceName     string
	Region           string
	AvailabilityZone string
	InstanceType     string
	MarketType       InstanceMarketType
	UserData         string
	Options          map[string]string
	Tags             []InstanceTag
	ProviderConfig   map[string]any
	ScopeID          string
}

type StartInstanceResult struct {
	StackName     string
	InstanceID    string
	URN           string
	PublicIP      string
	PrivateIP     string
	Warnings      []Warning
	LaunchFailure *LaunchFailure
}

type StopInstanceRequest struct {
	Context     RequestContext
	Credentials Credentials
	Scope       ConnectionScope
	StackName   string
	InstanceID  string
	Region      string
	Options     map[string]string
	ScopeID     string
}

type StopInstanceResult struct {
	InstanceID string
	Destroyed  bool
	Warnings   []Warning
}

type ListActiveInstancesRequest struct {
	Context           RequestContext
	Credentials       Credentials
	Scope             ConnectionScope
	Regions           []string
	AvailabilityZones []string
	InstanceTypes     []string
	Tags              []InstanceTag
	Options           map[string]string
}

type ActiveInstance struct {
	InstanceID         string
	Name               string
	Region             string
	AvailabilityZone   string
	InstanceType       string
	State              string
	MarketType         InstanceMarketType
	PublicIP           string
	PrivateIP          string
	IPv6Addresses      []string
	LaunchTime         time.Time
	Tags               []InstanceTag
	ProviderAttributes map[string]string
	ScopeID            string
}

type ListActiveInstancesResult struct {
	Items          []ActiveInstance
	Warnings       []Warning
	NextCursor     string
	CoveredRegions []string
}

type PurchaseOption string

const (
	PurchaseOptionOnDemand PurchaseOption = "on-demand"
	PurchaseOptionSpot     PurchaseOption = "spot"
)

type Region struct {
	Code string
	Name string
}

type AcceleratorKind string

const (
	AcceleratorKindGPU  AcceleratorKind = "gpu"
	AcceleratorKindFPGA AcceleratorKind = "fpga"
)

type Accelerator struct {
	Kind      AcceleratorKind
	Model     string
	Count     float64
	MemoryGiB float64
}

type LocalStorage struct {
	HasLocalStorage bool
	Medium          string
	DiskCount       int32
	TotalSizeGiB    float64
}

type InstanceTypeSummary struct {
	InstanceType         string
	Series               string
	Family               string
	Category             string
	DisplayName          string
	Generation           string
	VCPU                 int32
	MemoryGiB            float64
	Architectures        []string
	SupportedRegionCount int32
}

type InstanceTypeInfo struct {
	InstanceType              string
	Series                    string
	Family                    string
	Category                  string
	DisplayName               string
	Generation                string
	VCPU                      int32
	MemoryGiB                 float64
	Architectures             []string
	CPUManufacturer           string
	CPUModel                  string
	CPUClockSpeedGHz          string
	NetworkPerformance        string
	EnhancedNetworking        bool
	IPv6Supported             bool
	SupportedRegions          []Region
	SupportedOperatingSystems []string
	Accelerators              []Accelerator
	LocalStorage              *LocalStorage
	Attributes                map[string]string
}

type ListInstanceTypesRequest struct {
	Context       RequestContext
	Scope         ConnectionScope
	Region        string
	Series        []string
	InstanceTypes []string
	Architectures []string
	Generation    string
	Options       map[string]string
}

type ListInstanceTypesResult struct {
	Items    []InstanceTypeSummary
	Warnings []Warning
}

type GetInstanceTypeInfoRequest struct {
	Context       RequestContext
	Scope         ConnectionScope
	Region        string
	Series        []string
	InstanceTypes []string
	Options       map[string]string
}

type GetInstanceTypeInfoResult struct {
	Items    []InstanceTypeInfo
	Warnings []Warning
}

type GetInstancePricesRequest struct {
	Context              RequestContext
	Credentials          Credentials
	Scope                ConnectionScope
	Region               string
	InstanceTypes        []string
	PurchaseOption       PurchaseOption
	OperatingSystem      string
	Tenancy              string
	PreinstalledSoftware string
	LicenseModel         string
	Currency             string
	Options              map[string]string
}

type InstancePrice struct {
	InstanceType         string
	Region               Region
	PurchaseOption       PurchaseOption
	OperatingSystem      string
	Tenancy              string
	PreinstalledSoftware string
	LicenseModel         string
	BillingUnit          string
	Currency             string
	Price                string
	EffectiveAt          time.Time
	SKU                  string
	Description          string
}

type GetInstancePricesResult struct {
	Items    []InstancePrice
	Warnings []Warning
}

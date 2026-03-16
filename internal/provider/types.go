package provider

import "time"

type Cloud string

const (
	CloudAWS   Cloud = "aws"
	CloudAzure Cloud = "azure"
	CloudGCP   Cloud = "gcp"
)

type AuthScheme string

const (
	AuthSchemeAWSIAM            AuthScheme = "aws_iam"
	AuthSchemeAzureClientSecret AuthScheme = "azure_client_secret"
	AuthSchemeGCPServiceAccount AuthScheme = "gcp_service_account"
)

type Metadata struct {
	Name              string
	Version           string
	Cloud             Cloud
	SupportedAuth     []AuthScheme
	SupportedServices []string
	Capabilities      map[string]string
}

type RequestContext struct {
	RequestID string
	Caller    string
	TraceID   string
}

type ConnectionScope struct {
	AccountID      string
	Region         string
	Endpoint       string
	Attributes     map[string]string
	EndpointRegion string
}

type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	RoleARN         string
	ExternalID      string
}

type AzureCredentials struct {
	TenantID       string
	ClientID       string
	ClientSecret   string
	SubscriptionID string
}

type GCPCredentials struct {
	ProjectID    string
	ClientEmail  string
	PrivateKey   string
	PrivateKeyID string
}

type Credentials struct {
	AWS   *AWSCredentials
	Azure *AzureCredentials
	GCP   *GCPCredentials
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
	AMI              string
	InstanceType     string
	MarketType       InstanceMarketType
	SubnetID         string
	SecurityGroupIDs []string
	KeyName          string
	UserData         string
	Options          map[string]string
	Tags             []InstanceTag
}

type StartInstanceResult struct {
	StackName  string
	InstanceID string
	URN        string
	PublicIP   string
	PrivateIP  string
	Warnings   []Warning
}

type StopInstanceRequest struct {
	Context     RequestContext
	Credentials Credentials
	Scope       ConnectionScope
	StackName   string
	Options     map[string]string
}

type StopInstanceResult struct {
	StackName string
	Destroyed bool
	Warnings  []Warning
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
	InstanceID       string
	Name             string
	Region           string
	AvailabilityZone string
	InstanceType     string
	State            string
	MarketType       InstanceMarketType
	PublicIP         string
	PrivateIP        string
	IPv6Addresses    []string
	SubnetID         string
	VPCID            string
	LaunchTime       time.Time
	Tags             []InstanceTag
}

type ListActiveInstancesResult struct {
	Items    []ActiveInstance
	Warnings []Warning
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
	PlacementGroupSupported   bool
	VPCOnly                   bool
	EBSOptimized              bool
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

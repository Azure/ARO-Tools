package main

type CentralConfig struct {
	Settings    CloudSettings `json:"Settings"`
	Geographies []Geography   `json:"Geographies"`
}

// --- Settings Section ---

type CloudSettings struct {
	AAD                    AADSettings                    `json:"aad"`
	ActiveDirectory        ActiveDirectorySettings        `json:"activedirectory"`
	ADF                    ADFRegionConfig                `json:"adf"`
	ADGraph                ADGraphSettings                `json:"adgraph"`
	ADX                    ADXSettings                    `json:"adx"`
	ARM                    ARMSettings                    `json:"arm"`
	AzureContainerRegistry AzureContainerRegistrySettings `json:"azureContainerRegistry"`
	Billing                BillingSettings                `json:"billing"`
	CDN                    CDNSettings                    `json:"cdn"`
	CloudDns               string                         `json:"cloudDns"`
	CloudName              string                         `json:"cloudName"`
	CloudService           CloudServiceSettings           `json:"cloudService"`
	CosmosDB               CosmosDBSettings               `json:"cosmosDB"`
	DNS                    DNSSettings                    `json:"dns"`
	DSMS                   DSMSSettings                   `json:"dsms"`
	DSTS                   DSTSSettings                   `json:"dsts"`
	Entra                  EntraSettings                  `json:"entra"`
	EV2                    EV2Settings                    `json:"ev2"`
	EventHubs              EventHubsSettings              `json:"eventHubs"`
	FrontDoor              FrontDoorSettings              `json:"frontDoor"`
	Geneva                 GenevaSettings                 `json:"geneva"`
	Geo                    GeoSettings                    `json:"geo"`
	HDInsights             HDInsightsSettings             `json:"hdinsights"`
	Identity               IdentitySettings               `json:"identity"`
	KeyVault               KeyVaultSettings               `json:"keyVault"`
	Kusto                  KustoSettings                  `json:"kusto"`
	M365                   M365Settings                   `json:"m365"`
	Network                NetworkSettings                `json:"network"`
	O365                   O365Settings                   `json:"o365"`
	Redis                  RedisSettings                  `json:"redis"`
	ServiceBus             ServiceBusSettings             `json:"serviceBus"`
	SignalR                SignalRSettings                `json:"signalR"`
	SQL                    SQLSettings                    `json:"sql"`
	Storage                StorageSettings                `json:"storage"`
	TrafficManager         TrafficManagerSettings         `json:"trafficManager"`
	WebPubSub              WebPubSubSettings              `json:"webPubSub"`
	WebSites               WebSitesSettings               `json:"webSites"`
}

// --- AAD Section ---

type AADSettings struct {
	Endpoint        string                       `json:"endpoint"`
	GraphEndpoint   string                       `json:"graphEndpoint"`
	MsGraphEndpoint string                       `json:"msGraphEndpoint"`
	Tenants         map[string]AADTenantSettings `json:"tenants"`
}

type AADTenantSettings struct {
	EV2BuildoutServicePrincipalId string `json:"ev2BuildoutServicePrincipalId"`
	ID                            string `json:"id"`
	KeyVaultServicePrincipalId    string `json:"keyVaultServicePrincipalId"`
	WebAppRPServicePrincipalId    string `json:"webAppRPServicePrincipalId"`
}

// --- Active Directory Section ---

type ActiveDirectorySettings struct {
	Domain     ActiveDirectoryDomain            `json:"domain"`
	Federation ActiveDirectoryFederation        `json:"federation"`
	Tenants    map[string]ActiveDirectoryTenant `json:"tenants"`
}

type ActiveDirectoryDomain struct {
	Federation string `json:"federation"`
}

type ActiveDirectoryFederation struct {
	FQDN map[string]string `json:"fqdn"`
	Name map[string]string `json:"name"`
}

type ActiveDirectoryTenant struct {
	Management *ActiveDirectoryTenantInfo `json:"management,omitempty"`
	Operating  *ActiveDirectoryTenantInfo `json:"operating,omitempty"`
}

type ActiveDirectoryTenantInfo struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
}

// --- ADF Section ---

type ADFRegionConfig struct {
	DataPlaneEndpoint        string `json:"dataPlaneEndpoint"`
	RegionADFName            string `json:"regionADFName"`
	ResourceProviderEndpoint string `json:"resourceProviderEndpoint"`
}

// --- ADGraph Section ---

type ADGraphSettings struct {
	FQDN map[string]string `json:"fqdn"`
}

// --- ADX Section ---

type ADXSettings struct {
	Domain map[string]string `json:"domain"`
	FQDN   map[string]string `json:"fqdn"`
}

// --- ARM Section ---

type ARMSettings struct {
	AppIdUri             string            `json:"appIdUri"`
	AuthMetadataEndpoint string            `json:"authMetadataEndpoint"`
	Domain               map[string]string `json:"domain"`
	Endpoint             string            `json:"endpoint"`
	FQDN                 map[string]string `json:"fqdn"`
}

// --- Azure Container Registry Section ---

type AzureContainerRegistrySettings struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- Billing Section ---

type BillingSettings struct {
	Modern BillingModernSettings `json:"modern"`
}

type BillingModernSettings struct {
	Account BillingAccountSettings `json:"account"`
}

type BillingAccountSettings struct {
	Internal BillingInternalAccount `json:"internal"`
}

type BillingInternalAccount struct {
	AccountID        string `json:"accountid"`
	AccountWithOrgID string `json:"accountwithorgid"`
	OrgID            string `json:"orgid"`
}

// --- CDN Section ---

type CDNSettings struct {
	DNSSuffix        string `json:"dnsSuffix"`
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- Cloud Service Section ---

type CloudServiceSettings struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- CosmosDB Section ---

type CosmosDBSettings struct {
	CassandraDnsZone string `json:"cassandraDnsZone"`
	DnsSuffix        string `json:"dnsSuffix"`
	DomainNameSuffix string `json:"domainNameSuffix"`
	EtcdDnsZone      string `json:"etcdDnsZone"`
	GremlinDnsZone   string `json:"gremlinDnsZone"`
	MongoDnsZone     string `json:"mongoDnsZone"`
	SqlDnsZone       string `json:"sqlDnsZone"`
	TableDnsZone     string `json:"tableDnsZone"`
}

// --- DNS Section ---

type DNSSettings struct {
	AzClient   string `json:"azclient"`
	Azure      string `json:"azure"`
	CloudAPI   string `json:"cloudapi"`
	Microsoft  string `json:"microsoft"`
	MsIdentity string `json:"msidentity"`
}

// --- DSMS Section ---

type DSMSSettings struct {
	Domain         string            `json:"domain"`
	Endpoint       string            `json:"endpoint"`
	FQDN           map[string]string `json:"fqdn"`
	GlobalEndpoint string            `json:"globalEndpoint"`
	KustoCluster   DSMSKustoCluster  `json:"kustocluster"`
}

type DSMSKustoCluster struct {
	Primary DSMSKustoClusterPrimary `json:"primary"`
}

type DSMSKustoClusterPrimary struct {
	Hostname        string `json:"hostname"`
	Location        string `json:"location"`
	Name            string `json:"name"`
	NameAndLocation string `json:"nameandlocation"`
}

// --- DSTS Section ---

type DSTSSettings struct {
	Domain   string            `json:"domain"`
	Endpoint string            `json:"endpoint"`
	FQDN     map[string]string `json:"fqdn"`
	Realm    string            `json:"realm"`
}

// --- Entra Section ---

type EntraSettings struct {
	FederatedCredentials EntraFederatedCredentials `json:"federatedcredentials"`
	FQDN                 map[string]string         `json:"fqdn"`
	Tenants              map[string]EntraTenant    `json:"tenants"`
}

type EntraFederatedCredentials struct {
	Audience string `json:"audience"`
}

type EntraTenant struct {
	TenantDomain string `json:"tenantdomain"`
	TenantID     string `json:"tenantid"`
	TenantName   string `json:"tenantname"`
}

// --- EV2 Section ---

type EV2Settings struct {
	AppDeployIpAddresses []string            `json:"appDeployIpAddresses"`
	AuthResourceID       string              `json:"authresourceid"`
	BuildoutAppID        string              `json:"buildoutAppId"`
	Domain               map[string]string   `json:"domain"`
	Endpoint             string              `json:"endpoint"`
	FQDN                 map[string]string   `json:"fqdn"`
	IpAddresses          []string            `json:"ipAddresses"`
	ResourceURI          string              `json:"resourceUri"`
	RolloutAppID         string              `json:"rolloutAppId"`
	ServicePrincipal     EV2ServicePrincipal `json:"serviceprincipal"`
}

type EV2ServicePrincipal struct {
	Buildout map[string]EV2TenantPrincipal `json:"buildout"`
	Rollout  map[string]EV2TenantPrincipal `json:"rollout"`
}

type EV2TenantPrincipal struct {
	AppID         string `json:"appid"`
	ObjectID      string `json:"objectid"`
	TenantMoniker string `json:"tenantmoniker"`
}

// --- EventHubs Section ---

type EventHubsSettings struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- FrontDoor Section ---

type FrontDoorSettings struct {
	DNSSuffix        string `json:"dnsSuffix"`
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- Geneva Section ---

type GenevaSettings struct {
	Actions               GenevaActions         `json:"actions"`
	DeliveryService       GenevaDeliveryService `json:"deliveryservice"`
	Domain                map[string]string     `json:"domain"`
	GCSEnvironment        string                `json:"GCSEnvironment"`
	GlobalHealthEndpoint  string                `json:"globalHealthEndpoint"`
	Jarvis                GenevaJarvis          `json:"jarvis"`
	Logs                  GenevaLogs            `json:"logs"`
	MdsManagementEndpoint string                `json:"mdsManagementEndpoint"`
	Metrics               GenevaMetrics         `json:"metrics"`
}

type GenevaActions struct {
	DSTS            GenevaDSTS        `json:"dsts"`
	EnvironmentName map[string]string `json:"environmentname"`
	FQDN            map[string]string `json:"fqdn"`
	HomeDsts        map[string]string `json:"homeDsts"`
	SecondaryURL    string            `json:"secondaryUrl"`
	URL             string            `json:"url"`
}

type GenevaDSTS struct {
	ServiceAccount GenevaServiceAccount `json:"serviceaccount"`
	ServiceRealm   GenevaServiceRealm   `json:"servicerealm"`
}

type GenevaServiceAccount struct {
	Beta    GenevaServiceAccountInfo `json:"beta"`
	Primary GenevaServiceAccountInfo `json:"primary"`
}

type GenevaServiceAccountInfo struct {
	HomeDsts           string `json:"homedsts"`
	ServiceAccountName string `json:"serviceaccountname"`
}

type GenevaServiceRealm struct {
	Beta    map[string]GenevaServiceRealmInfo `json:"beta"`
	Primary map[string]GenevaServiceRealmInfo `json:"primary"`
}

type GenevaServiceRealmInfo struct {
	RedirectionURL string `json:"redirectionurl"`
	ServiceName    string `json:"servicename"`
	ServiceRealm   string `json:"servicerealm"`
}

type GenevaDeliveryService struct {
	ServicePrincipal GenevaDeliveryServicePrincipal `json:"serviceprincipal"`
}

type GenevaDeliveryServicePrincipal struct {
	CloudDiag map[string]EV2TenantPrincipal `json:"clouddiag"`
}

type GenevaJarvis struct {
	FQDN map[string]string `json:"fqdn"`
}

type GenevaLogs struct {
	FQDN           map[string]string `json:"fqdn"`
	GCSEnvironment string            `json:"gcsenvironment"`
}

type GenevaMetrics struct {
	FQDN map[string]string `json:"fqdn"`
}

// --- Geo Section ---

type GeoSettings struct {
	Cloud GeoCloudSettings `json:"cloud"`
}

type GeoCloudSettings struct {
	Domain map[string]string `json:"domain"`
}

// --- HDInsights Section ---

type HDInsightsSettings struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- Identity Section ---

type IdentitySettings struct {
	Domain map[string]string `json:"domain"`
	SLD    map[string]string `json:"sld"`
}

// --- KeyVault Section ---

type KeyVaultSettings struct {
	AppID                string   `json:"appId"`
	DnsSuffix            string   `json:"dnsSuffix"`
	DomainNameSuffix     string   `json:"domainNameSuffix"`
	ExtensionIpAddresses []string `json:"extensionIpAddresses"`
}

// --- Kusto Section ---

type KustoSettings struct {
	DnsSuffix           string `json:"dnsSuffix"`
	DomainNameSuffix    string `json:"domainNameSuffix"`
	MfaDomainNameSuffix string `json:"mfaDomainNameSuffix"`
}

// --- M365 Section ---

type M365Settings struct {
	EcsExtensionType string `json:"ecsExtensionType"`
}

// --- Network Section ---

type NetworkSettings struct {
	PublicIPAddress NetworkPublicIPAddress `json:"publicIPAddress"`
}

type NetworkPublicIPAddress struct {
	DNS string `json:"dns"`
}

// --- O365 Section ---

type O365Settings struct {
	Domain map[string]string `json:"domain"`
}

// --- Redis Section ---

type RedisSettings struct {
	DnsSuffix        string `json:"dnsSuffix"`
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- ServiceBus Section ---

type ServiceBusSettings struct {
	DnsSuffix        string `json:"dnsSuffix"`
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- SignalR Section ---

type SignalRSettings struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- SQL Section ---

type SQLSettings struct {
	Endpoint string            `json:"endpoint"`
	FQDN     map[string]string `json:"fqdn"`
}

// --- Storage Section ---

type StorageSettings struct {
	DnsSuffix        string `json:"dnsSuffix"`
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- TrafficManager Section ---

type TrafficManagerSettings struct {
	DnsSuffix        string `json:"dnsSuffix"`
	Domain           string `json:"domain"`
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- WebPubSub Section ---

type WebPubSubSettings struct {
	DomainNameSuffix string `json:"domainNameSuffix"`
}

// --- WebSites Section ---

type WebSitesSettings struct {
	DnsSuffix        string `json:"dnsSuffix"`
	DomainNameSuffix string `json:"domainNameSuffix"`
	WebAppRPAppID    string `json:"webAppRPAppId"`
}

// --- Geographies Section ---

type Geography struct {
	Name     string        `json:"Name"`
	Settings GeographyMeta `json:"Settings"`
	Regions  []Region      `json:"Regions"`
}

type GeographyMeta struct {
	GeoShortID string `json:"geoShortId"`
}

type Region struct {
	Name     string       `json:"Name"`
	Settings RegionConfig `json:"Settings"`
}

type RegionConfig struct {
	ADF                       *ADFRegionConfig   `json:"adf,omitempty"`
	AvailabilityZoneCount     int                `json:"availabilityZoneCount"`
	AvailabilityZoneLiveCount int                `json:"availabilityZoneLiveCount"`
	AvailabilityZones         []AvailabilityZone `json:"availabilityZones"`
	DcmtRegionID              string             `json:"dcmtRegionId"`
	PairedRegions             []string           `json:"pairedRegions"`
	RegionArchitecture        string             `json:"regionArchitecture"`
	RegionFriendlyName        string             `json:"regionFriendlyName"`
	RegionShortName           string             `json:"regionShortName"`
}

type AvailabilityZone struct {
	DcmtID string `json:"dcmtId"`
	IsLive bool   `json:"isLive"`
	State  string `json:"state"`
}

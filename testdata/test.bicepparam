// copy from dev-infrastructure/configurations/region.bicepparam
using '../templates/region.bicep'

// dns
param baseDNSZoneName = 'hcp.osadev.cloud'
param baseDNSZoneResourceGroup = 'global'

// CS
param csImage = '{{ .clusterService.imageTag }}'
param regionRG = '{{ .regionRG }}'

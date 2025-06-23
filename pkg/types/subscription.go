package types

type SubscriptionProvisioning struct {
	DisplayName                   Value  `json:"displayName"`
	AIRSRegisteredUserPrincipalId *Value `json:"airsRegisteredUserPrincipalId,omitempty"`
	CertificateDomains            *Value `json:"certificateDomains,omitempty"`
}

func (s *SubscriptionProvisioning) Validate() error {
	return nil
}

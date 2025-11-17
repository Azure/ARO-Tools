package ev2config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegionShortLowercase(t *testing.T) {
	ev2Config, err := readConfig()
	require.NoError(t, err)
	for cloud := range ev2Config.Clouds {
		for region := range ev2Config.Clouds[cloud].Regions {
			regionShortNameVal, ok := ev2Config.Clouds[cloud].Regions[region]["regionShortName"].(string)
			require.True(t, ok, "regionShortName is not a string for cloud %v, region %v", cloud, region)
			require.Equal(t, strings.ToLower(regionShortNameVal), regionShortNameVal, "regionShortName for cloud %v, region %v is not lowercase: got %v - see https://issues.redhat.com/browse/AROSLSRE-247 for more details", cloud, region, regionShortNameVal)
		}
	}
}

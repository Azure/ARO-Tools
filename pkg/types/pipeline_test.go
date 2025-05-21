package types

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/Azure/ARO-Tools/internal/testutil"
)

func TestNewPlainPipelineFromBytes(t *testing.T) {
	pipelineBytes, err := os.ReadFile("../../testdata/zz_fixture_TestNewPlainPipelineFromBytes.yaml")
	assert.NoError(t, err)

	p, err := NewPlainPipelineFromBytes("", pipelineBytes)
	assert.NoError(t, err)

	pipelineBytes, err = yaml.Marshal(p)
	assert.NoError(t, err)

	testutil.CompareWithFixture(t, pipelineBytes, testutil.WithExtension(".yaml"))

}

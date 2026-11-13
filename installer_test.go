package hotaru_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// The installer says the version the release is (spec 072 R6.4). .tag is
// the version of record, and fynstall.yaml is written by hand beside it.
func TestTheInstallerIsTheReleasesVersion(t *testing.T) {
	tag, err := os.ReadFile(".tag")
	require.NoError(t, err)
	body, err := os.ReadFile("fynstall.yaml")
	require.NoError(t, err)
	var config struct {
		App struct {
			Version string `yaml:"version"`
		} `yaml:"app"`
	}
	require.NoError(t, yaml.Unmarshal(body, &config))
	require.Equal(t, strings.TrimSpace(string(tag)), config.App.Version,
		"fynstall.yaml's app.version and .tag are the same string, or the release is wrong")
}

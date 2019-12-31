package grammar_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/abayer/jx-convert-jenkinsfile/pkg/grammar"

	"github.com/jenkins-x/jx/pkg/config"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestParsingGrammar(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name string
	}{
		{
			name: "basic",
		},
		{
			name: "script",
		},
		{
			name: "invalid_top_level",
		},
		{
			name: "invalid_stage_level",
		},
		{
			name: "invalid_when",
		},
		{
			name: "unsupported_step",
		},
		{
			name: "multiple_top_level_post_kind",
		},
		{
			name: "multiple_top_level_post_step",
		},
		{
			name: "nondefault_top_level_post_kind",
		},
		{
			name: "nondefault_top_level_post_step",
		},
	}

	for _, tt := range testCases {
		if tt.name != "invalid_stage_level" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			testDir := filepath.Join("test_data", "grammar", tt.name)
			_, err := os.Stat(testDir)
			assert.NoError(t, err)

			jf := filepath.Join(testDir, "Jenkinsfile")
			assert.NoError(t, err)

			model, err := grammar.ParseJenkinsfile(jf)
			assert.NoError(t, err)

			asYaml := model.ToYaml()
			t.Log("\n" + asYaml)

			yamlOutFile, err := ioutil.TempFile("", "test-grammar-jx-yml-")
			defer func() {
				err := util.DeleteFile(yamlOutFile.Name())
				assert.NoError(t, err)
			}()
			assert.NoError(t, err)

			err = ioutil.WriteFile(yamlOutFile.Name(), []byte(asYaml), 0600)
			assert.NoError(t, err)

			projectConfig, err := config.LoadProjectConfigFile(yamlOutFile.Name())
			assert.NoError(t, err)

			if projectConfig.PipelineConfig == nil {
				t.Fatal("No PipelineConfig")
			}

			assert.NotNil(t, projectConfig.PipelineConfig.Pipelines.PullRequest)
			assert.NotNil(t, projectConfig.PipelineConfig.Pipelines.PullRequest.Pipeline)
			assert.NotNil(t, projectConfig.PipelineConfig.Pipelines.Release)
			assert.NotNil(t, projectConfig.PipelineConfig.Pipelines.Release.Pipeline)
			parsed := projectConfig.PipelineConfig.Pipelines.Release.Pipeline

			validateErr := parsed.Validate(ctx)
			assert.Nil(t, validateErr, "validation error: %s", validateErr)

			expectedYamlBytes, err := ioutil.ReadFile(filepath.Join(testDir, "jenkins-x.yml"))
			assert.NoError(t, err)

			// Compare the expected YAML with our converted YAML (with an extra newline since that's just how IDEs tend to format YAML)
			assert.Equal(t, string(expectedYamlBytes), asYaml+"\n")
		})
	}
}

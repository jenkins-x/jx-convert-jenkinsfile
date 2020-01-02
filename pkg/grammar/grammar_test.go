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
		name          string
		convertIssues bool
	}{
		{
			name:          "basic",
			convertIssues: false,
		},
		{
			name:          "script",
			convertIssues: true,
		},
		{
			name:          "invalid_top_level",
			convertIssues: true,
		},
		{
			name:          "invalid_stage_level",
			convertIssues: true,
		},
		{
			name:          "invalid_when",
			convertIssues: true,
		},
		{
			name:          "unsupported_step",
			convertIssues: true,
		},
		{
			name:          "multiple_top_level_post_kind",
			convertIssues: true,
		},
		{
			name:          "multiple_top_level_post_step",
			convertIssues: true,
		},
		{
			name:          "nondefault_top_level_post_kind",
			convertIssues: true,
		},
		{
			name:          "nondefault_top_level_post_step",
			convertIssues: true,
		},
	}

	for _, tt := range testCases {
		if tt.name != "invalid_stage_level" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			testDir := filepath.Join("test_data", tt.name)
			_, err := os.Stat(testDir)
			assert.NoError(t, err)

			jf := filepath.Join(testDir, "Jenkinsfile")

			model, err := grammar.ParseJenkinsfile(jf)
			assert.NoError(t, err)

			asYaml, convertIssues, err := model.ToYaml()
			assert.NoError(t, err)

			if tt.convertIssues {
				assert.Equal(t, tt.convertIssues, convertIssues, "Expected to find conversion issues but there were none")
			} else {
				assert.Equal(t, tt.convertIssues, convertIssues, "Expected no conversion issues, but there were issues")
			}
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

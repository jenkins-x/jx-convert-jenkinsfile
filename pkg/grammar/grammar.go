package grammar

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/participle"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

const (
	indent              = "  "
	newlinePlaceholder  = "^^NEWLINE^^"
	backtickPlaceholder = "^^BACKTICK^^"
)

var (
	// Fields that are allowed but not translated in given contexts, resulting in warnings if used.
	unusedTopLevelFields = []string{
		"post",
	}
	unusedStageLevelFields = []string{
		"post",
	}

	// Fields that are explicitly unsupported in given contexts, resulting in errors if used.
	unsupportedTopLevelFields = []string{
		"triggers",
		"options",
		"parameters",
		"tools",
		"libraries",
	}
	unsupportedStageFields = []string{
		"stages",
		"parallel",
		"matrix",
		"tools",
		"input",
		"options",
	}

	// Fields that are explicitly supported in given contexts. Any other fields used in these contexts results in an error.
	supportedWhenFields = []string{
		"branch",
	}
	supportedSteps = []string{
		"sh",
		"dir",
		"container",
	}

	// Environment variables to remove from the Jenkinsfile
	unusedEnvVars = []string{
		"PREVIEW_VERSION",
		"APP_NAME",
		"DOCKER_REGISTRY",
		"DOCKER_REGISTRY_ORG",
	}

	// Steps from setVersion and setup that should be removed
	stepsToRemove = []string{
		"git checkout master",
		"checkout scm",
		"git config --global credential.helper store",
		"jx step git credentials",
		"echo \\$(jx-release-version) > VERSION",
		"mvn versions:set -DnewVersion=\\$(cat VERSION)",
		"jx step tag --version \\$(cat VERSION)",
	}
)

// Model is the base for the entire pipeline model
type Model struct {
	Pipeline []*ModelPipelineEntry `"pipeline" "{" { @@ } "}"`
}

func (m *Model) getPost() []*ModelPostEntry {
	for _, e := range m.Pipeline {
		if len(e.Post) > 0 {
			return e.Post
		}
	}
	return nil
}

func (m *Model) getEnvironment() []*ModelEnvironmentEntry {
	for _, e := range m.Pipeline {
		if len(e.Environment) > 0 {
			return e.Environment
		}
	}
	return nil
}

func (m *Model) getStages() []*ModelStage {
	for _, e := range m.Pipeline {
		if len(e.Stages) > 0 {
			return e.Stages
		}
	}
	return nil
}

func (m *Model) getUnsupported() []*UnsupportedModelBlock {
	for _, e := range m.Pipeline {
		if len(e.Unsupported) > 0 {
			return e.Unsupported
		}
	}
	return nil
}

// ToYaml converts the Jenkinsfile model into jenkins-x.yml
func (m *Model) ToYaml() string {
	// TODO: Everything that isn't stages

	var lines []string

	lines = append(lines, "buildPack: none")
	lines = append(lines, "pipelineConfig:")

	envLines := toEnvYamlLines(m.getEnvironment())
	if len(envLines) > 0 {
		lines = append(lines, indentLine("env:", 1))
		for _, envLine := range envLines {
			lines = append(lines, indentLine(envLine, 2))
		}
	}

	lines = append(lines, indentLine("pipelines:", 1))
	post := m.getPost()
	if len(post) > 1 || (len(post) == 1 && !post[0].isDefaultCleanWs()) {
		lines = append(lines, indentLine("# The Jenkinsfile contains a post directive for its pipeline. This is not converted.", 2))
		lines = append(lines, indentLine("# There is no equivalent behavior in Jenkins X pipelines.", 2))
	}
	for _, u := range m.getUnsupported() {
		lines = append(lines, indentLine(fmt.Sprintf("# The Jenkinsfile contains thex %s directive for its pipeline. This is not converted.", u.Name), 2))
		lines = append(lines, indentLine("# There is no equivalent behavior in Jenkins X pipelines.", 2))
	}

	var releaseStages []*ModelStage
	var prStages []*ModelStage
	allStages := m.getStages()

	for _, s := range allStages {
		when := s.getWhen()
		if when == nil {
			releaseStages = append(releaseStages, s)
			prStages = append(prStages, s)
		} else if when.Branch == "master" {
			releaseStages = append(releaseStages, s)
		} else if strings.HasPrefix(when.Branch, "PR-") {
			prStages = append(prStages, s)
		} else if len(when.Unsupported) > 0 {
			for _, u := range when.Unsupported {
				lines = append(lines, indentLine(fmt.Sprintf("# This Jenkinsfile contains the unsupported when condition '%s' on stage '%s'. The stage containing it will not be converted.", u.Name, s.Name), 2))
			}
		}

		post := s.getPost()
		if len(post) > 0 {
			lines = append(lines, indentLine(fmt.Sprintf("# The Jenkinsfile contains a post directive for the stage '%s'. This is not converted.", s.Name), 2))
			lines = append(lines, indentLine("# There is no equivalent behavior in Jenkins X pipelines.", 2))
		}

		for _, u := range s.getUnsupported() {
			lines = append(lines, indentLine(fmt.Sprintf("# The Jenkinsfile contains the %s directive for the stage '%s'. This is not converted.", u.Name, s.Name), 2))
			lines = append(lines, indentLine("# There is no equivalent behavior in Jenkins X pipelines.", 2))
		}
	}

	lines = append(lines, prOrReleasePipelineAsYAML(prStages, false))
	lines = append(lines, prOrReleasePipelineAsYAML(releaseStages, true))

	return strings.Join(lines, "\n")
}

func prOrReleasePipelineAsYAML(stages []*ModelStage, isRelease bool) string {
	var lines []string
	stepCount := 0

	envVars := make(map[string]*ModelEnvironmentEntry)
	var stepLines []string
	image := ""

	var pipelineType string
	var stageName string
	var longTypeName string
	if isRelease {
		pipelineType = "release"
		stageName = "Release Build"
		longTypeName = "release"
	} else {
		pipelineType = "pullRequest"
		stageName = "PR Build"
		longTypeName = "pull request"
	}

	lines = append(lines, indentLine(pipelineType+":", 2))
	lines = append(lines, indentLine("pipeline:", 3))
	lines = append(lines, indentLine("stages:", 4))
	lines = append(lines, indentLine("- name: "+stageName, 5))

	for _, s := range stages {
		stageImage, stageSteps := s.toImageAndSteps()
		if image == "" {
			image = stageImage
		}
		// Deduplicate env vars
		for _, env := range s.getEnvironment() {
			if _, ok := envVars[env.Key]; !ok && env.Key != "" {
				envVars[env.Key] = env
			}
		}
		stepLines = append(stepLines, stageSteps...)
	}
	lines = append(lines, indentLine("agent:", 6))
	if image == "" {
		image = "maven"
	}
	lines = append(lines, indentLine(fmt.Sprintf("image: %s", image), 7))
	var envList []*ModelEnvironmentEntry
	for _, envVar := range envVars {
		envList = append(envList, envVar)
	}
	envYamlLines := toEnvYamlLines(envList)
	if len(envYamlLines) > 0 {
		lines = append(lines, indentLine("environment:", 6))
		for _, l := range envYamlLines {
			lines = append(lines, indentLine(l, 7))
		}
	}
	lines = append(lines, indentLine("steps:", 6))
	if len(stepLines) == 0 {
		lines = append(lines, indentLine("# No stages were found that will be run in the "+longTypeName+" pipeline.", 7))
		lines = append(lines, indentLine("- name: step0", 7))
		lines = append(lines, indentLine("sh: echo 'No "+longTypeName+" stages found, failing' && exit 1", 8))
	}
	for _, l := range stepLines {
		lines = append(lines, indentLine(fmt.Sprintf("- name: step%d", stepCount), 7))
		lines = append(lines, l)
		stepCount++
	}

	return strings.Join(lines, "\n")
}

// UnsupportedModelBlock represents a field that is unsupported and will cause an error.
type UnsupportedModelBlock struct {
	Name  string `@Ident`
	Value string `@RawString`
}

// ToString converts the model to a rough string form
func (m *UnsupportedModelBlock) ToString() string {
	return fmt.Sprintf("UNSUPPORTED: %s %s", m.Name, toCurlyStringFromEscaped(m.Value))
}

// ModelPipelineEntry represents the directives that can be contained within the pipeline block
type ModelPipelineEntry struct {
	Agent       *ModelAgent              `"agent" "{" @@ "}"`
	Environment []*ModelEnvironmentEntry `| "environment" "{" { @@ } "}"`
	Stages      []*ModelStage            `| "stages" "{" { @@ } "}"`
	Post        []*ModelPostEntry        `| "post" "{" { @@ } "}"`
	Unsupported []*UnsupportedModelBlock `| @@`
}

// ModelAgent represents the agent block in Declarative
type ModelAgent struct {
	Label string `"label" @String`
}

// ToString converts the model to a rough string form
func (m *ModelAgent) ToString() string {
	return fmt.Sprintf("agent label: %s", m.Label)
}

// ModelEnvironmentEntry represents a `foo = bar` (or `foo = credentials("bar")` in the environment block
type ModelEnvironmentEntry struct {
	Key   string                      `@Ident`
	Value *ModelEnvironmentEntryValue `"=" @@`
}

func toEnvYamlLines(modelVars []*ModelEnvironmentEntry) []string {
	var envVars []corev1.EnvVar
	for _, e := range modelVars {
		envVars = append(envVars, e.ToJXEnv()...)
	}
	if len(envVars) == 0 {
		return nil
	}
	// TODO: Error handling probably
	envYamlBytes, err := yaml.Marshal(envVars)
	if err != nil {
		log.Logger().Errorf("err: %s", err)
	}
	// Trim off the last line of "    \n" if it's there.
	envYaml := strings.TrimSpace(string(envYamlBytes))
	return strings.Split(envYaml, "\n")
}

// ToJXEnv converts to jenkins-x.yml friendly environment variables
func (m *ModelEnvironmentEntry) ToJXEnv() []corev1.EnvVar {
	for _, e := range unusedEnvVars {
		if m.Key == e {
			return nil
		}
	}
	// TODO: Warning comment for values with $
	if m.Value.StringValue != nil && strings.Contains(*m.Value.StringValue, "$") {
		return nil
	}
	if m.Value.Credential != nil {
		// Special handling of CHARTMUSEUM_CREDS
		if m.Key == "CHARTMUSEUM_CREDS" {
			return []corev1.EnvVar{
				{
					Name: "CHARTMUSEUM_CREDS_USR",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: *m.Value.Credential,
							},
							Key: "BASIC_AUTH_USER",
						},
					},
				},
				{
					Name: "CHARTMUSEUM_CREDS_PSW",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: *m.Value.Credential,
							},
							Key: "BASIC_AUTH_PASS",
						},
					},
				},
			}
		}
	}

	return []corev1.EnvVar{{
		Name:  m.Key,
		Value: *m.Value.StringValue,
	}}
}

// ModelEnvironmentEntryValue represents either a string or a credentials step's value
type ModelEnvironmentEntryValue struct {
	StringValue *string `  @(String|Char)`
	Credential  *string `| "credentials" "(" @(String|Char) ")"`
}

// ToString converts the model to a rough string form
func (m *ModelEnvironmentEntryValue) ToString() string {
	if m.StringValue != nil {
		return *m.StringValue
	}
	if m.Credential != nil {
		return *m.Credential
	}
	return "n/a"
}

// ModelStage represents a stage in a Jenkinsfile
type ModelStage struct {
	Name    string             `"stage" "(" @String ")"`
	Entries []*ModelStageEntry `"{" { @@ } "}"`
}

// toImageAndSteps converts the model to jenkins-x.yml representation
func (m *ModelStage) toImageAndSteps() (string, []string) {
	// TODO: Unsupported post, input et al

	var stepLines []string

	var baseSteps []stepDirAndImage

	// Use the maven pod template as a default
	image := "maven"

	if len(m.getSteps()) > 0 && m.getSteps()[0].Name == "container" {
		image = m.getSteps()[0].getArg()
	}
	for _, s := range m.getSteps() {
		baseSteps = append(baseSteps, s.nestedStepsWithDirAndImage("", image)...)
	}

	var stepsToInclude []stepDirAndImage

	// Filter out setVersion and setup steps
	for _, s := range baseSteps {
		if !s.step.shouldRemove() {
			stepsToInclude = append(stepsToInclude, s)
		}
	}

	for _, s := range stepsToInclude {
		var singleStep []string
		if s.step.Name == "sh" {
			if len(s.step.Args) != 1 {
				// TODO: Something about how the other sh parameters don't translate
			}
			arg := s.step.Args[0]
			if arg.Unnamed == nil {
				// TODO: Something about how named arguments don't translate
			}
			singleStep = append(singleStep, indentLine(fmt.Sprintf("sh: %s", s.step.getJxArg()), 8))

			if s.image != image {
				singleStep = append(singleStep, indentLine(fmt.Sprintf("image: %s", s.image), 8))
			}
			if s.dir != "" {
				singleStep = append(singleStep, indentLine(fmt.Sprintf("dir: %s", s.dir), 8))
			}
		} else {
			// Not a valid step, so add a boilerplate "echo 'step (name) can't be translated' && exit 1" sh, and a
			// comment with the original text
			singleStep = append(singleStep, indentLine(fmt.Sprintf("# The Jenkins Pipeline step %s cannot be translated directly.", s.step.Name), 8))
			singleStep = append(singleStep, indentLine("# You may want to consider adding a shell script to your repository that replicates its behavior.", 8))
			singleStep = append(singleStep, indentLine("# Original step from Jenkinsfile:", 8))
			for _, l := range strings.Split(s.step.toOriginalGroovy(), "\n") {
				singleStep = append(singleStep, indentLine("# "+l, 8))
			}
			singleStep = append(singleStep, indentLine(fmt.Sprintf("sh: echo 'Invalid step %s, failing' && exit 1", s.step.Name), 8))
		}
		if len(singleStep) > 0 {
			stepLines = append(stepLines, strings.Join(singleStep, "\n"))
		}
	}

	return image, stepLines
}

func indentLine(line string, count int) string {
	return fmt.Sprintf("%s%s", strings.Repeat(indent, count), line)
}
func (m *ModelStage) getEnvironment() []*ModelEnvironmentEntry {
	for _, e := range m.Entries {
		if len(e.Environment) > 0 {
			return e.Environment
		}
	}
	return nil
}

func (m *ModelStage) getUnsupported() []*UnsupportedModelBlock {
	for _, e := range m.Entries {
		if len(e.Unsupported) > 0 {
			return e.Unsupported
		}
	}
	return nil
}

func (m *ModelStage) getSteps() []*ModelStep {
	for _, e := range m.Entries {
		if len(e.Steps) > 0 {
			return e.Steps
		}
	}
	return nil
}

func (m *ModelStage) getWhen() *ModelWhen {
	for _, e := range m.Entries {
		if e.When != nil {
			return e.When
		}
	}
	return nil
}

func (m *ModelStage) getPost() []*ModelPostEntry {
	for _, e := range m.Entries {
		if len(e.Post) > 0 {
			return e.Post
		}
	}
	return nil
}

// ModelStageEntry represents the various directives contained within a stage
type ModelStageEntry struct {
	Agent       *ModelAgent              `  "agent" "{" @@ "}"`
	Environment []*ModelEnvironmentEntry `| "environment" "{" { @@ } "}"`
	Steps       []*ModelStep             `| "steps" "{" { @@ } "}"`
	Post        []*ModelPostEntry        `| "post" "{" { @@ } "}"`
	When        *ModelWhen               `| "when" "{" @@ "}"`
	Unsupported []*UnsupportedModelBlock `| @@`
}

// ModelWhen represents a when block - only branch is supported currently
type ModelWhen struct {
	Branch      string                   `"branch" @String`
	Unsupported []*UnsupportedModelBlock `| @@`
}

// ToString converts the model to a rough string form
func (m *ModelWhen) ToString() string {
	return fmt.Sprintf("when: branch %s", m.Branch)
}

// ModelPostEntry represents a post condition and its steps
type ModelPostEntry struct {
	Kind  string       `@Ident`
	Steps []*ModelStep `"{" { @@ } "}"`
}

func (m *ModelPostEntry) isDefaultCleanWs() bool {
	if m.Kind == "always" && len(m.Steps) == 1 {
		s := m.Steps[0]
		return s.Name == "cleanWs" && len(s.Args) == 0
	}
	return false
}

// ModelStep represents either a normal step or a script block
type ModelStep struct {
	Name        string          `@Ident`
	Args        []*ModelStepArg `"("? @@? { "," @@ } ")"?`
	NestedSteps []*ModelStep    `("{" { @@ } "}")*`
}

type stepDirAndImage struct {
	step  *ModelStep
	dir   string
	image string
}

func (m *ModelStep) nestedStepsWithDirAndImage(baseDir string, baseImage string) []stepDirAndImage {
	var steps []stepDirAndImage

	if len(m.NestedSteps) == 0 {
		steps = append(steps, stepDirAndImage{
			step:  m,
			dir:   baseDir,
			image: baseImage,
		})
	} else {
		if m.Name == "dir" {
			baseDir = strings.Trim(m.getArg(), "./")
		} else if m.Name == "container" {
			baseImage = m.getArg()
		}
		for _, s := range m.NestedSteps {
			steps = append(steps, s.nestedStepsWithDirAndImage(baseDir, baseImage)...)
		}
	}
	return steps
}

func (m *ModelStep) getJxArg() string {
	rawArg := m.getArg()
	catWithDollarSign := regexp.MustCompile(`\\\$\(cat .*?VERSION\)`)
	catWithBackticks := regexp.MustCompile("`cat VERSION`")

	fixedArg := catWithDollarSign.ReplaceAllString(rawArg, "${inputs.params.version}")
	fixedArg = catWithBackticks.ReplaceAllString(fixedArg, "${inputs.params.version}")

	return fixedArg
}

func (m *ModelStep) getArg() string {
	if len(m.Args) == 1 {
		return strings.Trim(m.Args[0].ToString(), "\"")
	}
	return ""
}

func (m *ModelStep) shouldRemove() bool {
	if len(m.Args) == 1 && m.Name == "sh" {
		for _, n := range stepsToRemove {
			if strings.Trim(m.Args[0].ToString(), "\"") == n {
				return true
			}
		}
	}
	return false
}

// ToString converts the model to a rough string form
func (m *ModelStep) ToString() string {
	var entries []string
	if m.Name == "script" && len(m.Args) == 1 {
		entries = append(entries, fmt.Sprintf("script is unsupported: %s", toCurlyStringFromEscaped(m.Args[0].Unnamed.ToString())))
	} else {
		entries = append(entries, fmt.Sprintf("name: %s", m.Name))
		if len(m.Args) > 0 {
			entries = append(entries, "args:")
			for _, e := range m.Args {
				entries = append(entries, "\t"+e.ToString())
			}
		}
		if len(m.NestedSteps) > 0 {
			entries = append(entries, fmt.Sprintf("nested steps (%d):", len(m.NestedSteps)))
			for _, e := range m.NestedSteps {
				entries = append(entries, e.ToString())
				entries = append(entries, fmt.Sprintf("%+v", e))
			}
		}
	}
	return strings.Join(entries, "\n")
}

func (m *ModelStep) toOriginalGroovy() string {
	var lines []string
	if len(m.NestedSteps) == 0 {
		if len(m.Args) == 0 {
			lines = append(lines, fmt.Sprintf("%s()", m.Name))
		} else if len(m.Args) == 1 {
			arg := m.Args[0]
			if arg.Unnamed != nil {
				if strings.Contains(m.getArg(), newlinePlaceholder) {
					// Convert the escaped string back into groovy and use that
					lines = append(lines, fmt.Sprintf("%s %s", m.Name, toCurlyStringFromEscaped(m.getArg())))
				} else {
					lines = append(lines, fmt.Sprintf("%s %s", m.Name, m.getArg()))
				}
			} else {
				// There's one named argument, which is weird, but ok.
				lines = append(lines, fmt.Sprintf("%s(%s: %s)", m.Name, arg.Named.Key, arg.Named.Value.ToString()))
			}
		} else {
			var argStrings []string
			for _, a := range m.Args {
				if a.Unnamed != nil {
					argStrings = append(argStrings, a.Unnamed.ToString())
				} else if a.Named != nil {
					argStrings = append(argStrings, fmt.Sprintf("%s: %s", a.Named.Key, a.Named.Value.ToString()))
				}
			}
			lines = append(lines, fmt.Sprintf("%s(%s)", m.Name, strings.Join(argStrings, ", ")))
		}
	} else {
		// TODO: Something about nested steps? I think we covered all of these already via the escaping magic...
	}

	// TODO: Probably should be splitting toCurlyStringFromEscaped above rather than joining everything and resplitting where we call this but for now? eh.
	return strings.Join(lines, "\n")
}

// ModelStepArg represents an argument to a step
type ModelStepArg struct {
	Unnamed *Value             `  @@`
	Named   *ModelStepNamedArg `| @@`
}

// ToString converts the model to a rough string form
func (m *ModelStepArg) ToString() string {
	if m.Unnamed != nil {
		return m.Unnamed.ToString()
	}
	if m.Named != nil {
		return m.Named.ToString()
	}
	return "(none)"
}

type ModelStepNamedArg struct {
	Key   string `@(Ident|String|Char)`
	Value *Value `":" @@`
}

// ToString converts the model to a rough string form
func (m *ModelStepNamedArg) ToString() string {
	return fmt.Sprintf("key: %s, val: %s", m.Key, m.Value.ToString())
}

type Value struct {
	String *string  `  @(String|RawString)`
	Number *float64 `| @Float`
	Int    *int64   `| @Int`
	Bool   *bool    `| (@"true" | "false")`
}

// ToString converts the model to a rough string form
func (v *Value) ToString() string {
	if v.String != nil {
		return "\"" + *v.String + "\""
	}
	if v.Number != nil {
		return fmt.Sprintf("%d", v.Number)
	}
	if v.Int != nil {
		return fmt.Sprintf("%d", v.Int)
	}
	if v.Bool != nil {
		return fmt.Sprintf("%t", *v.Bool)
	}

	return "n/a"
}

// ParseJenkinsfileInDirectory looks for a Jenkinsfile in a directory and parses it
func ParseJenkinsfileInDirectory(dir string) (*Model, error) {
	dirExists, err := doesDirExist(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "Error checking if %s is a directory", dir)
	}
	if !dirExists {
		return nil, fmt.Errorf("The directory %s does not exist or is not a directory", dir)
	}

	jf := filepath.Join(dir, "Jenkinsfile")
	fileExists, err := doesFileExist(jf)
	if err != nil {
		return nil, errors.Wrapf(err, "Error checking if %s is a file", jf)
	}
	if !fileExists {
		return nil, fmt.Errorf("The file %s does not exist or is not a file", jf)
	}

	return ParseJenkinsfile(jf)
}

// doesFileExist checks if path exists and is a file
func doesFileExist(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, errors.Wrapf(err, "failed to check if file exists %s", path)
}

// doesDirExist checks if path exists and is a directory
func doesDirExist(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir(), nil
	} else if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// ParseJenkinsfile takes a Jenkinsfile and returns the resulting model
func ParseJenkinsfile(jenkinsfile string) (*Model, error) {
	jf, err := ioutil.ReadFile(jenkinsfile)
	if err != nil {
		return nil, err
	}

	replacedJF := strings.ReplaceAll(string(jf), "\\$", "\\\\$")
	replacedJF = strings.ReplaceAll(replacedJF, ".toLowerCase()", "")

	curlyBlocks := GetBlocks(replacedJF)
	for _, b := range curlyBlocks {
		replacedJF = escapeUnsupportedFieldsInContext(b, "steps", supportedSteps, replacedJF, false)
		replacedJF = escapeUnsupportedFieldsInContext(b, "when", supportedWhenFields, replacedJF, false)
		replacedJF = escapeUnsupportedFieldsInContext(b, "stage", unsupportedStageFields, replacedJF, true)
		replacedJF = escapeUnsupportedFieldsInContext(b, "pipeline", unsupportedTopLevelFields, replacedJF, true)
	}

	parser, err := participle.Build(&Model{})
	if err != nil {
		return nil, err
	}
	model := &Model{}
	err = parser.ParseString(replacedJF, model)

	if err != nil {
		log.Logger().Errorf("err parsing: %s", err)
		return nil, errors.Wrapf(err, "Jenkinsfile %s cannot be translated", jenkinsfile)
	}

	return model, nil
}

func escapeUnsupportedFieldsInContext(block curlyBlock, context string, fields []string, jfText string, isBlacklist bool) string {
	if block.Name == context {
		for _, nested := range block.Nested {
			if !isSupportedField(nested.Name, fields, isBlacklist) {
				jfText = strings.ReplaceAll(jfText, nested.OriginalText, nested.ReplacementText)
			}
		}
	}
	return jfText
}

func toEscapedFromCurlyString(curly string) string {
	wsPrefix := ""
	wsRegexp := regexp.MustCompile(`^(\s+)\S`)
	var indentRemoved []string
	for _, l := range strings.Split(curly, "\n") {
		if l != "" && wsPrefix == "" {
			match := wsRegexp.FindStringSubmatch(l)
			if match[1] != "" {
				wsPrefix = match[1]
				if len(wsPrefix) > 2 {
					wsPrefix = wsPrefix[2:]
				}
			}
		}
		indentRemoved = append(indentRemoved, strings.TrimPrefix(l, wsPrefix))
	}
	escaped := strings.Join(indentRemoved, newlinePlaceholder)
	escaped = strings.ReplaceAll(escaped, "`", backtickPlaceholder)
	return escaped
}

func toCurlyStringFromEscaped(escaped string) string {
	curly := strings.ReplaceAll(escaped, newlinePlaceholder, "\n")
	curly = strings.ReplaceAll(curly, "\\\\", "\\")
	curly = strings.ReplaceAll(curly, backtickPlaceholder, "`")
	return "{" + curly + "}"
}

type curlyBlock struct {
	Name            string
	Nested          []curlyBlock
	OriginalText    string
	ReplacementText string
}

func (cb curlyBlock) ToString() string {
	lines := []string{fmt.Sprintf("name: %s, containing...", cb.Name)}
	if len(cb.Nested) > 0 {
		for _, n := range cb.Nested {
			lines = append(lines, fmt.Sprintf(" - %s", n.Name))
		}
	} else {
		lines = append(lines, " - just text")
	}
	return strings.Join(lines, "\n")
}

func GetBlocks(fullString string) []curlyBlock {

	var blocks []curlyBlock

	var re = regexp.MustCompile(`(\w+)(\(.*?\))?\s+{`)

	for _, matchingIdx := range re.FindAllStringSubmatchIndex(fullString, -1) {
		// Start with the name - matchingIdx[2]:matchingIdx[3] is the submatch's index
		block := curlyBlock{
			Name: fullString[matchingIdx[2]:matchingIdx[3]],
		}
		// Now get a substring from right after the curly brace (at matchingIdx[1]) until end of the full string
		fromCurly := fullString[matchingIdx[1]:]

		// Set curlyCount to 1, for the curly at matchingIdx[1]-1 (i.e., before the start of fromCurly)
		curlyCount := 1

		// init a var for the closing curly index
		var closingIndex int

		// Check each character until we get the closing curly
		for inCurlyIdx, c := range fromCurly {
			if c == '{' {
				curlyCount++
			}
			if c == '}' {
				curlyCount--
			}
			if curlyCount == 0 {
				closingIndex = inCurlyIdx
				break
			}
		}

		// Set the block's content to the full match up to and including the closing curly
		block.OriginalText = fullString[matchingIdx[0]:matchingIdx[1]] + fromCurly[:closingIndex+1]

		// Set the replacement text, in case it's needed. That'll be everything but the opening curly and closing curly
		// in the original text, which will be replaced with backticks, and with the contents of the block being escaped.
		block.ReplacementText = fullString[matchingIdx[0]:matchingIdx[1]-1] + "`" + toEscapedFromCurlyString(fromCurly[:closingIndex]) + "`"

		// Get any nested for the content within the curlies
		block.Nested = GetBlocks(fromCurly[:closingIndex-1])

		// Add the block to the list
		blocks = append(blocks, block)
	}

	return blocks
}

func isSupportedField(name string, fields []string, isBlacklist bool) bool {
	for _, f := range fields {
		if name == f {
			// If the list of fields is a blacklist, and we've found the name, return false
			if isBlacklist {
				return false
			} else {
				// If the list of fields is a blacklist and we've found the name, return true
				return true
			}
		}
	}

	// If we haven't found the name at all, then return isBlacklist - that'll be true for blacklists, meaning if we haven't
	// found the name, the field is allowed, while it'll be false for whitelists, meaning if we haven't found the name,
	// the field is not allowed.
	return isBlacklist
}

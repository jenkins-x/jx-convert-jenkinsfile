package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/jenkins-x/jx-convert-jenkinsfile/pkg/grammar"
)

func main() {

	dir := flag.String("dir", ".", "the folder to look for a Jenkinsfile and to write the jenkins-x.yml. Defaults to the current directory.")

	flag.Parse()

	model, err := grammar.ParseJenkinsfileInDirectory(*dir)

	if err != nil {
		fmt.Println("Error parsing Jenkinsfile: ", err)
		os.Exit(1)
	}

	asYaml, convertIssues, err := model.ToYaml()
	if err != nil {
		fmt.Println("Error converting jenkins-x.yml: ", err)
		os.Exit(1)
	}
	jxYmlFile := filepath.Join(*dir, "jenkins-x.yml")
	err = ioutil.WriteFile(jxYmlFile, []byte(asYaml), 0644)
	if err != nil {
		fmt.Printf("Error writing to jenkins-x.yml in %s: %s\n", *dir, err)
		os.Exit(1)
	}

	fmt.Printf("Converted jenkins-x.yml for Jenkinsfile in %s:\n", *dir)
	if convertIssues {
		fmt.Println("ATTENTION: Some contents of the Jenkinsfile could not be converted. Please review the jenkins-x.yml for more information.")
	}
}

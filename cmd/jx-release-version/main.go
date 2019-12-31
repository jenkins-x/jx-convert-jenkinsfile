package jx_release_version

import (
	"flag"
	"fmt"
	"os"

	"github.com/abayer/jx-convert-jenkinsfile/pkg/grammar"
)

func main() {

	dir := flag.String("dir", ".", "the folder to look for a Jenkinsfile. Defaults to the current directory.")

	flag.Parse()

	model, err := grammar.ParseJenkinsfileInDirectory(*dir)

	if err != nil {
		fmt.Println("Error parsing Jenkinsfile: ", err)
		os.Exit(1)
	}

	asYaml := model.ToYaml()
	fmt.Printf("Converted jenkins-x.yml for Jenkinsfile in %s:\n", *dir)
	fmt.Println("====================")
	fmt.Println(asYaml)
	fmt.Println("====================")
}


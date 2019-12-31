package main

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

	asYaml, err := model.ToYaml()
	if err != nil {
		fmt.Println("Error generating jenkins-x.yml: ", err)
		os.Exit(1)
	}
	fmt.Printf("Converted jenkins-x.yml for Jenkinsfile in %s:\n", *dir)
	fmt.Println("====================")
	fmt.Println(asYaml)
	fmt.Println("====================")
}

module github.com/abayer/jx-convert-jenkinsfile

go 1.12

require (
	github.com/alecthomas/participle v0.4.1
	github.com/blang/semver v3.5.1+incompatible
	github.com/jenkins-x/jx v0.0.0-20191206153325-7bde8a5b439a
	github.com/pkg/errors v0.8.1
	github.com/stretchr/testify v1.4.0
	k8s.io/api v0.0.0-20190816222004-e3a6b8045b0b
	k8s.io/apimachinery v0.0.0-20190816221834-a9f1d8a9c101
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/metrics v0.0.0-20190704050707-780c337c9cbd // indirect
	sigs.k8s.io/yaml v1.1.0
)

replace k8s.io/api => k8s.io/api v0.0.0-20181128191700-6db15a15d2d3

replace k8s.io/metrics => k8s.io/metrics v0.0.0-20181128195641-3954d62a524d

replace k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190122181752-bebe27e40fb7

replace k8s.io/client-go => k8s.io/client-go v2.0.0-alpha.0.0.20190115164855-701b91367003+incompatible

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20181128195303-1f84094d7e8e

- [Lazyload E2E Test Tutorials](#lazyload-e2e-test-tutorials)
	- [Overview](#overview)
	- [slime/slime-framework/test/e2e File Introduction](#slimeslime-frameworkteste2e-file-introduction)
	- [[slime_module]/test/e2e  File Introduction](#slime_moduleteste2e--file-introduction)
	- [Usage](#usage)
		- [Method I Ginkgo](#method-i-ginkgo)
			- [Install](#install)
			- [Compile and Execute](#compile-and-execute)
			- [Sample results](#sample-results)
		- [Method II Go Test](#method-ii-go-test)
			- [Compile](#compile)
			- [Execute](#execute)
			- [Sample results](#sample-results-1)


# Lazyload E2E Test Tutorials

## Overview

In order to make module test, slime bring in E2E(End to End) test framework. The main functions implementation refers to Kubernetes test/e2e.

The test framework is located in the path **slime-framework/test/e2e** of [Slime project](https://github.com/slime-io/slime). The test cases are located in each module project. 



## slime/slime-framework/test/e2e File Introduction

| Path        | Usage                                                        |
| ----------- | ------------------------------------------------------------ |
| ./common/   | Specifies the version information of each base image and automatically replaces the relevant content in the test file |
| ./framework | Framework layer, provides a variety of e2e basic functions, such as logging, command line parameters passing, framework initialization, file reading |

You can supplement the framework by yourself, and it is recommended to refer to the k8s implementation as a priority.



## [slime_module]/test/e2e  File Introduction

| Path          | Usage                              |
| ------------- | ---------------------------------- |
| ./testdata    | test resource files                |
| ./e2e_test.go | main entrance, function is TestE2E |
| ./xxx_test.go | test cases                         |

Users can write their own xxx_test.go under test/e2e to enrich the test cases.



## Usage

There are two ways to compile and execute E2E test cases, ginkgo and go test. We recommend using the more convenient and flexible ginkgo.



### Method I Ginkgo

Ginkgo is a Go testing framework. Slime's E2E testing is based on ginkgo implementation, which supports Ginkgo command line and can organize test cases according to ginkgo usage.

[Ginkgo Official Tutorial](https://onsi.github.io/ginkgo/)



#### Install

```sh
$ go get github.com/onsi/ginkgo/ginkgo
```



#### Compile and Execute

Ginkgo will compile and execute the test cases with one command, and delete the compiled binaries after the test is finished, very convenient. The parameters available on the command line are described in slime-framework/test/e2e/framework/text_context.go.



When testing, it will be very convenient to run a subset of spec. Let's assume a test case as follows, Describe spec contains three It specs.

```go
var _ = ginkgo.Describe("Slime e2e test", func() {
	// ...

	ginkgo.It("clean resource", func() {
		// ...
	})

	ginkgo.It("rev lazyload works", func() {
		// ...
	})

	ginkgo.It("no-rev lazyload works", func() {
		// ...
	})
})
```



In the test case directory, run all tests

```sh
$ ginkgo
```



Run the specified spec, supporting regular expressions and parameters such as focus, skip, etc., for example

```sh
#run specs that starts with rev
$ ginkgo --focus=rev

#skip specs that starts with rev
$ ginkgo --skip=rev
```



You can also add F before It or Describe to indicate execution, or P or X to indicate ignore, so that you don't have to pass parameters on the command line.

Details at [Ginkgo Cli](https://onsi.github.io/ginkgo/#the-ginkgo-cli) and [Ginkgo Spec Runner](https://onsi.github.io/ginkgo/#the-spec-runner)



#### Sample results

```sh
$ ginkgo --focus=clean
Oct 27 09:27:30.112: INFO: >>> kubeConfig: 
Oct 27 09:27:30.112: ERROR: load kubeconfig failed unable to load in-cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined
Oct 27 09:27:30.112: INFO: Starting e2e run "4d0b5d24-44b8-46cc-8bf2-6db425acfa2e" on ginkgo node 1 

Running Suite: e2e test suite
=============================
Random Seed: 1635298037
Will run 1 of 3 specs

•SS
JUnit report was created: /home/hazard/go/src/lazyload/test/e2e/reports/service_01.xml

Ran 1 of 3 Specs in 1.208 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 2 Skipped
PASS

Ginkgo ran 1 suite in 14.005721903s
Test Suite Passed
```





### Method II Go Test

#### Compile

```sh
$ cd [module]/test/e2e
$ go test -c
```

The compiled binary file is the e2e.test file.



#### Execute

```sh
$ cd slime/modules/[module]/test/e2e
$ ./e2e.test
```

The parameters available for command line execution are described in slime-framework/test/e2e/framework/text_context.go



#### Sample results

```sh
$ ./e2e.test
Oct 11 15:24:20.490: INFO: >>> kubeConfig: 
Oct 11 15:24:20.490: ERROR: load kubeconfig failed unable to load in-cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined
Oct 11 15:24:20.491: INFO: Starting e2e run "46bc5af1-23b3-4557-9815-47d0512367b2" on ginkgo node 1 

Running Suite: e2e test suite
=============================
Random Seed: 1633937060
Will run 1 of 1 specs

• [SLOW TEST:151.511 seconds]
Slime e2e test
/home/hazard/go/src/slime/slime-modules/lazyload/test/e2e/lazyload_test.go:33
  slime module lazyload works
  /home/hazard/go/src/slime/slime-modules/lazyload/test/e2e/lazyload_test.go:37
------------------------------

JUnit report was created: /home/hazard/go/src/slime/slime-modules/lazyload/test/e2e/reports/service_01.xml

Ran 1 of 1 Specs in 151.511 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 0 Skipped
PASS
```

By default, the test report overview is generated under slime-modules/[module]/test/e2e/reports, and the path can be changed by passing report-dir during execution.








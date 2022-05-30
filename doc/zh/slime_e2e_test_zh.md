- [Lazyload E2E 测试教程](#lazyload-e2e-测试教程)
	- [概述](#概述)
	- [slime/slime-framework/test/e2e下文件说明](#slimeslime-frameworkteste2e下文件说明)
	- [[slime_module]/test/e2e下文件说明](#slime_moduleteste2e下文件说明)
	- [使用](#使用)
		- [方法一 Ginkgo](#方法一-ginkgo)
			- [安装](#安装)
			- [编译执行](#编译执行)
			- [结果样例](#结果样例)
		- [方法二 Go Test](#方法二-go-test)
			- [编译](#编译)
			- [执行](#执行)
			- [结果样例](#结果样例-1)


# Lazyload E2E 测试教程

## 概述

为了更好的进行模块化测试，Slime引入了E2E(End to End)测试框架。主要函数实现参考了Kubernetes的test/e2e。由于Kubernetes的E2E测试太过庞大，我们摘取了一个子集，满足基本需求。

测试框架位于[Slime项目](https://github.com/slime-io/slime)的slime-framework/test/e2e路径下，测试文件则位于各个modules项目中。以[lazyload项目](../../staging/src/slime.io/slime/modules/lazyload)为例，测试文件路径为test/e2e/lazyload_test.go。



## slime/slime-framework/test/e2e下文件说明

| Path        | Usage                                                        |
| ----------- | ------------------------------------------------------------ |
| ./common/   | 主要指定了各个基础镜像的版本信息，并自动替换测试文件中的相关内容 |
| ./framework | 框架层，提供了各种e2e基础函数，例如日志、命令行传参、framework初始化、文件读取 |

如果需要的功能，框架空缺，可以自行补充framework，建议优先参考k8s的实现。



## [slime_module]/test/e2e下文件说明

| Path          | Usage                 |
| ------------- | --------------------- |
| ./testdata    | 测试资源文件          |
| ./e2e_test.go | 主入口，函数为TestE2E |
| ./xxx_test.go | 具体模块测试用例      |

用户可自行在test/e2e下编写xxx_test.go，丰富测试用例。



## 使用

E2E测试用例的编译执行有两种方法，ginkgo和go test。我们推荐使用更为方便灵活的ginkgo。



### 方法一 Ginkgo

Ginkgo是一个 Go 测试框架，Slime的E2E测试是基于ginkgo实现的，支持Ginkgo命令行，可以按照ginkgo的用法组织测试用例。

[Ginkgo官方教程](https://ke-chain.github.io/ginkgodoc/)



#### 安装

```sh
$ go get github.com/onsi/ginkgo/ginkgo
```



#### 编译执行

Ginkgo会一键编译执行测试用例，并在测试结束后删除编译二进制文件，非常便捷。命令行执行时可用参数说明，参见slime-framework/test/e2e/framework/text_context.go



当开发的时候，运行 spec的子集将会非常方便。我们假设一个测试用例如下，Describe spec包含了三个It spec。

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



在测试用例目录下，运行全部测试

```sh
$ ginkgo
```



运行指定spec，支持正则表达和focus, skip等参数，比如

```sh
#运行rev开头测试
$ ginkgo --focus=rev

#跳过rev开头测试
$ ginkgo --skip=rev
```



也可以在It或Describe前增加F表示执行，或者增加P或X表示忽略，这样就不用命令行传参了。

详见[Ginkgo Cli](https://ke-chain.github.io/ginkgodoc/#ginkgo-cli)和[Ginkgo Spec 执行器](https://ke-chain.github.io/ginkgodoc/#spec-%E6%89%A7%E8%A1%8C%E5%99%A8)



#### 结果样例

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





### 方法二 Go Test

#### 编译

```sh
$ cd [module]/test/e2e
$ go test -c
```

编译后的二进制文件为e2e.test文件



#### 执行

```sh
$ cd slime/modules/[module]/test/e2e
$ ./e2e.test
```

命令行执行时可用参数说明，参见slime-framework/test/e2e/framework/text_context.go



#### 结果样例

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

默认会在slime-modules/[module]/test/e2e/reports下生成测试报告概述，可以执行时传递report-dir改变路径。








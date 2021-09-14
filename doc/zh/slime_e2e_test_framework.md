# Slime E2E 测试框架介绍



为了更好的进行模块化测试，Slime引入了E2E(End to End)测试框架。测试框架位于test/e2e路径下，主要函数实现参考了Kubernetes的test/e2e。由于Kubernetes的E2E测试太过庞大，Slime只摘取了很小的一个子集，满足基本功能需求。



## test/e2e下文件说明

| Path          | Usage                                                        |
| ------------- | ------------------------------------------------------------ |
| ./common/     | 主要指定了各个基础镜像的版本信息，并自动替换测试文件中的相关内容 |
| ./framework   | 框架层，提供了各种e2e基础函数，例如日志、命令行传参、framework初始化、文件读取 |
| ./testdata    | 测试文件                                                     |
| ./e2e_test.go | 主入口，函数为TestE2E                                        |
| ./xxx_test.go | 具体模块测试用例                                             |

用户可自行在test/e2e下编写xxx_test.go，丰富测试用例。如果需要的功能，框架空缺，可以自行补充framework，建议优先参考k8s的实现。





## 编译

```sh
$ cd test/e2e
$ go test -c
```

编译后的二进制文件为test/e2e/e2e.test文件





## 执行

```sh
$ cd test/e2e
$ ./e2e.test
```

命令行执行时可用参数说明，参见test/e2e/framework/text_context.go





## 结果

```sh
$ ./e2e.test
Sep 14 10:56:56.775: INFO: >>> kubeConfig: 
Sep 14 10:56:56.775: ERROR: load kubeconfig failed unable to load in-cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined
Sep 14 10:56:56.775: INFO: Starting e2e run "23dd1494-9c42-4e15-a696-04e7f7e8e934" on ginkgo node 1 

Running Suite: e2e test suite
=============================
Random Seed: 1631588216
Will run 1 of 1 specs

• [SLOW TEST:103.286 seconds]
Slime e2e test
/home/hazard/go/src/slime/test/e2e/lazyload_test.go:34
  slime module lazyload works
  /home/hazard/go/src/slime/test/e2e/lazyload_test.go:38
------------------------------

JUnit report was created: /home/hazard/go/src/slime/test/e2e/reports/service_01.xml

Ran 1 of 1 Specs in 103.286 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 0 Skipped
PASS

$ cat reports/service_01.xml 
<?xml version="1.0" encoding="UTF-8"?>
  <testsuite name="e2e test suite" tests="1" failures="0" errors="0" time="103.286">
      <testcase name="Slime e2e test slime module lazyload works" classname="e2e test suite" time="103.28572514"></testcase>
```

默认会在test/e2e/reports下生成测试报告概述，可以执行时传递report-dir改变路径。








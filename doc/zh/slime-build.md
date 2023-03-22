- [slime 镜像构建](#slime-镜像构建)
  - [子模块单独构建镜像](#子模块单独构建镜像)
    - [构建amd64镜像](#构建amd64镜像)
    - [构建多架构镜像](#构建多架构镜像)
  - [构建包含全部功能的slime镜像](#构建包含全部功能的slime镜像)
    - [构建amd64镜像](#构建amd64镜像-1)
    - [构建多架构镜像](#构建多架构镜像-1)

# slime 镜像构建

slime 支持子模块单独构建镜像,也支持构建一个包含全部功能的bundle镜像, 以下分两个部分介绍slime镜像构建

**注意：为了支持多架构镜像,slime用buildx构建镜像, 所以需要所在机器支持`docker buildx`**

## 子模块单独构建镜像

### 构建amd64镜像

切换至子模块所在目录,如限流模块所在目录 `slime/staging/src/slime.io/slime/modules/limiter`

note: 对于 global-sidecar, 需要切换至`slime/staging/src/slime.io/slime/modules/lazyload/cmd/proxy`

运行以下命令构建amd64镜像

```sh
./publish.sh build image
```

运行以下命令构建arm64镜像

```sh
TARGET_GOARCH=arm64 ./publish.sh build image
```

以上命令中使用到了publish脚本，它提供了以下参数 `build`,`image`,`image-push`和`ALL`

1. build: 用于构建可执行文件

2. image: 用于在本地出镜像,可以修改`slime/bin/publish`中的默认的`hub`地址

3. image-push: 将镜像推送至镜像仓库

4. ALL: 以上三个个步骤的结合

如，构建镜像并推至默认厂库

```sh
./publish.sh build image image-push

# 等同于 ./publish.sh ALL
```

### 构建多架构镜像

切换至子模块所在目录,运行以下命令构建多架构镜像,默认hub地址在 `slime/bin/publish`中定义

```sh
./publish.sh publish amd64 arm64
```

## 构建包含全部功能的slime镜像

### 构建amd64镜像

切换至 slime/staging/src/slime.io/slime/modules/bundle-all 目录, 运行以下命令构建bundle的amd64镜像

```sh
./publish.sh build image
```

### 构建多架构镜像

切换至 slime/staging/src/slime.io/slime/modules/bundle-all 目录,运行以下命令构建多架构镜像

```sh
./publish.sh publish amd64 arm64
```
- [Overview](#overview)
  - [Background](#background)
  - [Features](#features)
  - [Function](#function)
  - [Thinking](#thinking)
  - [Architecture](#architecture)
  - [Sample](#sample)
  - [Dependencies](#dependencies)
# Overview

[中文](./README_ZH.md)

## Background

In Service Mesh, in order to configure rate limit for a service, users have to face an unusually complex `EnvoyFilter` rate limit configuration. To solve this problem, this project introduces `SmartLimiter`, which can automatically convert user-submitted `SmartLimiter` into `EnvoyFilter`. [Installation and Use](./document/smartlimiter.md#installation-and-usage)

## Features

1. easy to use, just submit `SmartLimiter` to achieve the purpose of service rate limit.
2. adaptive rate limit, dynamic triggering of rate limit rules according to `metric`. 
3. Cover many scenarios, support global shared rate limit, global average rate limit, and single rate limit.

## Function
1. single rate limit, each load of the service will have its own rate limit counter. 
2. global shared rate limit, all loads of a service share a single rate limit counter. 
3. global average rate limit, which distributes the rate limit counters equally among all loads.
see [function](./document/smartlimiter.md#smartlimiter)

## Thinking

To get users out of the tedious `EnvoyFilter` configuration, we define an easy `API` using `kubernetes` `CRD` mechanism, the `SmartLimiter` resource within `kubernetes`. After a user submits a `SmartLimiter` to a `kubernetes` cluster, the `SmartLimiter Controler` generates an `EnvoyFilter` in conjunction with the `SmartLimiter` spec and service metrics.

## Architecture

The main architecture of adaptive rate limit is divided into two parts, one part includes the logical transformation of `SmartLimiter` to `EnvoyFilter`, and the other part includes the acquisition of monitoring data within the cluster, including service metrics such as `CPU`, `Memory`, `POD` counts, etc., as detailed in [architecture]()

<img src="./media/SmartLimiter.png" style="zoom:80%;" />

## Sample

When the total amount of `cpu` consumed by all loads of the `reviews` service is greater than 10, trigger a rate limit so that each load's port 9080 can only handle 10 requests per second, see [example](./document/smartlimiter.md#example)

~~~yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: review
  namespace: default
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 1
          quota: "10"
          strategy: "single"
        condition: "{{._base.cpu.sum}}>10"
        target:
          port: 9080
~~~

## Dependencies

1. In order to complete the adaptive function, we need to get the basic metrics of the service, so this service depends on `prometheus`, for details on how to build a simple `prometheus`, see [prometheus](./document/smartlimiter.md#installing-prometheus)
2. In order to complete the global shared rate limitation, we need a global counter, we introduced `RLS`, about `RLS` see [RLS](./document/smartlimiter.md#installing-rls--redis)


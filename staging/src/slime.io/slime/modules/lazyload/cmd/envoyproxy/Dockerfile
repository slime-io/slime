FROM envoyproxy/envoy:v1.26-latest

WORKDIR /

COPY proxy.yaml . 

ENTRYPOINT ["envoy","-c","./proxy.yaml"]

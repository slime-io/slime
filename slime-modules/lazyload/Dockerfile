# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM ubuntu:bionic
WORKDIR /
COPY manager.exe ./manager

ENTRYPOINT ["/manager"]
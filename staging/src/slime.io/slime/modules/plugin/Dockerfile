# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
ARG BASE_IMAGE
FROM ${BASE_IMAGE}

WORKDIR /
COPY manager.exe ./manager

ENTRYPOINT ["/manager"]
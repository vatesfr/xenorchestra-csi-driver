# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

################################################################################
##                               BUILD ARGS                                   ##
################################################################################
# This build arg allows the specification of a custom Golang image.
ARG GOLANG_IMAGE=golang:1.24-alpine

# This build arg allows the specification of a custom base image.
ARG BASE_IMAGE=alpine

################################################################################
##                              BUILD STAGE                                   ##
################################################################################
# Build the manager as a statically compiled binary so it has no dependencies
# libc, muscl, etc.
FROM ${GOLANG_IMAGE} AS builder
RUN apk update && apk add --no-cache make

# This build arg is the version to embed in the CSI binary
ARG VERSION
ARG GIT_COMMIT

WORKDIR /build
COPY go.mod go.sum ./
COPY Makefile ./
COPY pkg/    pkg/
COPY cmd/    cmd/
RUN VERSION="${VERSION}" GIT_COMMIT="${GIT_COMMIT}" make build-all-archs


################################################################################
##                               MAIN STAGE                                   ##
################################################################################
FROM --platform=${TARGETARCH} ${BASE_IMAGE} AS release

# This build arg is the git commit to embed in the CSI binary
ARG GIT_COMMIT

# This label will be overridden from driver base image
LABEL git_commit=$GIT_COMMIT
LABEL "maintainers"="Vates.tech <admin@vates.tech>" 

RUN apk add util-linux coreutils socat tar e2fsprogs && apk update && apk upgrade

# Remove cached data
RUN apk cache clean
ARG TARGETARCH
COPY --from=builder /build/bin/xenorchestra-csi-${TARGETARCH} /bin/xenorchestra-csi

ENTRYPOINT ["/bin/xenorchestra-csi"]
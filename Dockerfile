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

# Build the binary
FROM --platform=${BUILDPLATFORM} golang:1.19 AS builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.work go.work
COPY go.work.sum go.work.sum

COPY go.mod go.mod
COPY go.sum go.sum

COPY camel-k/go.mod camel-k/go.mod
COPY camel-k/go.sum camel-k/go.sum
COPY camel-k/pkg/apis/camel/go.mod camel-k/pkg/apis/camel/go.mod
COPY camel-k/pkg/apis/camel/go.sum camel-k/pkg/apis/camel/go.sum
COPY camel-k/pkg/client/camel/go.mod camel-k/pkg/client/camel/go.mod
COPY camel-k/pkg/client/camel/go.sum camel-k/pkg/client/camel/go.sum
COPY camel-k/pkg/kamelet/repository/go.mod camel-k/pkg/kamelet/repository/go.mod
COPY camel-k/pkg/kamelet/repository/go.sum camel-k/pkg/kamelet/repository/go.sum

COPY controller-runtime/go.mod controller-runtime/go.mod
COPY controller-runtime/go.sum controller-runtime/go.sum

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the sources
COPY cmd/ cmd/
COPY pkg/ pkg/

# Copy sub-modules
COPY camel-k/ camel-k/
COPY controller-runtime/ controller-runtime/

COPY Makefile Makefile
RUN mkdir bin

ARG TARGETOS
ARG TARGETARCH

RUN make OS=${TARGETOS} ARCH=${TARGETARCH} build

FROM eclipse-temurin:11

ARG MAVEN_VERSION="3.8.4"
ARG MAVEN_HOME="/usr/share/maven"
ARG SHA="a9b2d825eacf2e771ed5d6b0e01398589ac1bfa4171f36154d1b5787879605507802f699da6f7cfc80732a5282fd31b28e4cd6052338cbef0fa1358b48a5e3c8"
ARG BASE_URL="https://archive.apache.org/dist/maven/maven-3/${MAVEN_VERSION}/binaries"

USER 0

RUN mkdir -p ${MAVEN_HOME} \
    && curl -Lso /tmp/maven.tar.gz ${BASE_URL}/apache-maven-${MAVEN_VERSION}-bin.tar.gz \
    && echo "${SHA} /tmp/maven.tar.gz" | sha512sum -c - \
    && tar -xzC ${MAVEN_HOME} --strip-components=1 -f /tmp/maven.tar.gz \
    && rm -v /tmp/maven.tar.gz \
    && ln -s ${MAVEN_HOME}/bin/mvn /usr/bin/mvn \
    && rm ${MAVEN_HOME}/lib/maven-slf4j-provider*

ADD camel-k/build/_kamelets /kamelets
COPY camel-k/build/_maven_overlay/ /usr/share/maven/lib/
ADD camel-k/build/_maven_overlay/logback.xml /usr/share/maven/conf/

ENV MAVEN_OPTS="${MAVEN_OPTS} -Dlogback.configurationFile=/usr/share/maven/conf/logback.xml"

RUN mkdir -p /tmp/artifacts/m2 \
    && chgrp -R 0 /tmp/artifacts/m2 \
    && chmod -R g=u /tmp/artifacts/m2 \
    && chgrp -R 0 /kamelets \
    && chmod -R g=u /kamelets

WORKDIR /
COPY --from=builder workspace/bin/* /
USER 65532:65532

ENTRYPOINT ["/camel-kcp"]

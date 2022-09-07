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
FROM golang:1.18 AS builder

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
COPY pkg/ pkg/
COPY cmd/ cmd/

# Copy sub-modules
COPY camel-k/ camel-k/
COPY controller-runtime/ controller-runtime/

COPY Makefile Makefile
RUN mkdir bin
RUN CGO_ENABLED=0 make build

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder workspace/bin/* /
USER 65532:65532

ENTRYPOINT ["/camel-kcp"]

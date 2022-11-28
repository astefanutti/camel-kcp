# camel-kcp

[Camel K](https://github.com/apapche/camel-k) on [kcp](https://github.com/kcp-dev/kcp).

# Testing

## Prerequisites

* [Make](https://www.gnu.org/software/make)
* [Go](https://go.dev/doc/install) (v1.18 recommended)
* [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)
* [Docker](https://docs.docker.com/get-docker)
* [KinD](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
* [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)

## Setup

You can run the following command to set up kcp and KinD clusters:

```console
$ make local-setup
```

## Run

### Local

Once kcp setup, you can run camel-kcp locally, by running the following command in another terminal:

```console
$ KUBECONFIG=.kcp/admin.kubeconfig ./bin/camel-kcp --config=./config/deploy/local/config.yaml
```

### Deploy

Another alternative is to deploy camel-kcp in kcp itself, by running the following command in another terminal:

```console
$ make local-deploy
```

## Test

Once camel-kcp is running, you can exercise it using one of the methods below.

### Manual

It is recommended to install the kcp plugin, by running the following command once:

```console
$ go install ./cmd/kubectl-kcp
```

Then, switch to using the `user` context and workspace, by running:

```console
$ export KUBECONFIG=.kcp/admin.kubeconfig
$ kubectl config use-context user
$ kubectl kcp ws
```

Then, create a child workspace, with Camel K ready to use:

```console
$ kubectl kcp ws create demo --type camel-k --enter
```

Finally, create an integration, e.g. by running:

```console
$ cat <<EOF | kubectl apply -f -
apiVersion: camel.apache.org/v1
kind: Integration
metadata:
  name: hello
spec:
  flows:
    - from:
        uri: platform-http:/hello
        steps:
          - transform:
              simple: Hello ${header.name} \n\n ${headers}
          - to: log:info
  traits:
    health:
      enabled: true
```

Alternatively, you can using the `run` command of the Camel K CLI.

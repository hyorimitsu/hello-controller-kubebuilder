hello-controller-kubebuilder
---

Implement a custom controller for k8s.

## Description

Using [kubebuilder](https://book.kubebuilder.io/), implement a controller that manages `Foo`, a upper-level resource for `Deployment`.

See the diagram on [this page](https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md) for a description of how the custom controller works.

## Usage

Generate CRD manifest and apply.

```shell
make install
```

If you want to run this on local Go.

```shell
make run
```

If you want to run this on Container.

```shell
make deploy
```
